package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webhook-service/internal/api"
	"webhook-service/internal/dispatcher"
	"webhook-service/internal/health"
	"webhook-service/internal/store"
	"webhook-service/internal/webhook"

	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	port         = ":8084"
	dbPath       = "webhook.db"
	maxBodySize  = 10 * 1024 * 1024
	workerCount  = 10
	queueSize    = 1000
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().Msg("starting webhook service...")

	s, err := store.NewStore(dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create store")
	}
	defer s.Close()
	log.Info().Msg("store initialized")

	verifier := webhook.NewSignatureVerifier(s)
	log.Debug().Msg("signature verifier initialized")

	disp := dispatcher.NewDispatcher(s, workerCount, queueSize)
	disp.Start()
	defer disp.Stop()
	log.Info().Int("workers", workerCount).Msg("dispatcher started")

	healthChecker := health.NewChecker(s)

	webhookHandler := webhook.NewHandler(s, verifier, disp, maxBodySize)
	apiHandler := api.NewHandler(s, healthChecker, disp)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})

	webhookHandler.RegisterRoutes(r)
	apiHandler.RegisterRoutes(r)

	scheduler := gocron.NewScheduler(time.UTC)

	_, err = scheduler.Every(1).Minute().Do(func() {
		log.Debug().Msg("running health check job")
		eps, err := s.ListEndpoints()
		if err != nil {
			log.Error().Err(err).Msg("health check: failed to list endpoints")
			return
		}
		active := 0
		paused := 0
		disabled := 0
		for _, ep := range eps {
			switch ep.Status {
			case "active":
				active++
			case "paused":
				paused++
			case "disabled":
				disabled++
			}
		}
		log.Info().
			Int("total", len(eps)).
			Int("active", active).
			Int("paused", paused).
			Int("disabled", disabled).
			Msg("health check summary")
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to schedule health check job")
	}

	scheduler.StartAsync()
	log.Info().Msg("scheduler started")
	defer scheduler.Stop()

	srv := &http.Server{
		Addr:    port,
		Handler: r,
	}

	go func() {
		log.Info().Str("port", port).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited")
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		log.Info().
			Str("method", method).
			Str("path", path).
			Int("status", statusCode).
			Str("client_ip", clientIP).
			Dur("latency", latency).
			Msg("request")
	}
}
