package webhook

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"webhook-service/internal/model"
	"webhook-service/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	DefaultMaxBodySize = 10 * 1024 * 1024
)

type Handler struct {
	store       *store.Store
	verifier    *SignatureVerifier
	dispatcher  Dispatcher
	maxBodySize int64
}

type Dispatcher interface {
	Dispatch(event *model.Event)
}

func NewHandler(store *store.Store, verifier *SignatureVerifier, dispatcher Dispatcher, maxBodySize int64) *Handler {
	if maxBodySize <= 0 {
		maxBodySize = DefaultMaxBodySize
	}
	return &Handler{
		store:       store,
		verifier:    verifier,
		dispatcher:  dispatcher,
		maxBodySize: maxBodySize,
	}
}

func (h *Handler) ReceiveWebhook(c *gin.Context) {
	signature := c.GetHeader("X-Signature")
	keyID := c.GetHeader("X-Key-ID")
	timestampStr := c.GetHeader("X-Timestamp")
	eventID := c.GetHeader("X-Event-ID")
	eventType := c.GetHeader("X-Event-Type")
	sourceIP := c.ClientIP()

	if eventType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Event-Type header"})
		return
	}

	if eventID == "" {
		eventID = uuid.New().String()
	}

	duplicate, err := h.store.IsDuplicateEvent(eventID)
	if err != nil {
		log.Error().Err(err).Msg("failed to check duplicate event")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if duplicate {
		c.JSON(http.StatusOK, gin.H{"status": "received", "event_id": eventID, "duplicate": true})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBodySize)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	timestamp, err := ParseTimestamp(timestampStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timestamp"})
		return
	}

	verifyResult := h.verifyByAllEndpoints(body, timestamp, signature, keyID)

	event := &model.Event{
		ID:          eventID,
		EventType:   eventType,
		Payload:     body,
		SourceIP:    sourceIP,
		Signature:   signature,
		KeyID:       keyID,
		Timestamp:   timestamp,
		SignatureOK: verifyResult.Valid,
		ReceivedAt:  time.Now().UTC(),
	}

	err = h.store.MarkEventReceived(eventID)
	if err != nil {
		log.Error().Err(err).Msg("failed to mark event received")
	}
	err = h.store.CreateEvent(event)
	if err != nil {
		log.Error().Err(err).Msg("failed to create event")
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "received",
		"event_id":   eventID,
		"duplicate":  false,
		"signature_ok": verifyResult.Valid,
	})

	if verifyResult.Valid {
		h.dispatcher.Dispatch(event)
	}
}

type verifyRes struct {
	Valid bool
}

func (h *Handler) verifyByAllEndpoints(payload []byte, timestamp int64, signature string, keyID string) verifyRes {
	endpoints, err := h.store.ListEndpoints()
	if err != nil {
		log.Error().Err(err).Msg("failed to list endpoints")
		return verifyRes{Valid: false}
	}

	for _, ep := range endpoints {
		for _, key := range ep.APIKeys {
			if key.KeyID == keyID {
				result := h.verifier.VerifyWithKeys(ep.APIKeys, payload, timestamp, signature, keyID)
				if result.Valid {
					return verifyRes{Valid: true}
				}
			}
		}
	}

	return verifyRes{Valid: false}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/webhook/receive", h.ReceiveWebhook)
}

func (h *Handler) VerifyPayload(payload []byte, timestamp int64, signature string, keyID string, endpointID string) (bool, string) {
	result := h.verifier.VerifyEndpoint(endpointID, payload, timestamp, signature, keyID)
	return result.Valid, result.Error
}

var _ = bytes.NewReader
