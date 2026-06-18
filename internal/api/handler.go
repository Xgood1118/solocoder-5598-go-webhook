package api

import (
	"net/http"
	"time"

	"webhook-service/internal/health"
	"webhook-service/internal/model"
	"webhook-service/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	store        *store.Store
	healthChecker *health.Checker
	dispatcher   Dispatcher
}

type Dispatcher interface {
	RetryDeadLetter(dlID string) error
}

func NewHandler(store *store.Store, healthChecker *health.Checker, dispatcher Dispatcher) *Handler {
	return &Handler{
		store:         store,
		healthChecker: healthChecker,
		dispatcher:    dispatcher,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		endpoints := api.Group("/endpoints")
		{
			endpoints.GET("", h.ListEndpoints)
			endpoints.POST("", h.CreateEndpoint)
			endpoints.GET("/:id", h.GetEndpoint)
			endpoints.PUT("/:id", h.UpdateEndpoint)
			endpoints.DELETE("/:id", h.DeleteEndpoint)
			endpoints.POST("/:id/status", h.UpdateEndpointStatus)
			endpoints.GET("/:id/health", h.GetEndpointHealth)
			endpoints.GET("/:id/deliveries", h.ListEndpointDeliveries)
			endpoints.POST("/:id/keys", h.AddAPIKey)
			endpoints.DELETE("/:id/keys/:keyId", h.DeleteAPIKey)
		}

		events := api.Group("/events")
		{
			events.GET("/:id", h.GetEvent)
		}

		deadLetters := api.Group("/dead-letters")
		{
			deadLetters.GET("", h.ListDeadLetters)
			deadLetters.GET("/:id", h.GetDeadLetter)
			deadLetters.POST("/:id/retry", h.RetryDeadLetter)
			deadLetters.POST("/batch-retry", h.BatchRetryDeadLetters)
			deadLetters.POST("/:id/resolve", h.ResolveDeadLetter)
			deadLetters.DELETE("/:id", h.DeleteDeadLetter)
		}

		health := api.Group("/health")
		{
			health.GET("", h.SystemHealth)
			health.GET("/endpoints", h.AllEndpointsHealth)
		}
	}
}

type CreateEndpointRequest struct {
	Name                  string              `json:"name" binding:"required"`
	URL                   string              `json:"url" binding:"required"`
	EventTypes            []string            `json:"event_types" binding:"required"`
	RetryPolicy           *model.RetryPolicy  `json:"retry_policy"`
	SlowConsumerThreshold int64               `json:"slow_consumer_threshold_ms"`
	SlowConsumerMaxCount  int                 `json:"slow_consumer_max_count"`
}

func (h *Handler) CreateEndpoint(c *gin.Context) {
	var req CreateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ep := &model.Endpoint{
		ID:                    uuid.New().String(),
		Name:                  req.Name,
		URL:                   req.URL,
		EventTypes:            req.EventTypes,
		APIKeys:               []model.APIKeyPair{},
		Status:                model.EndpointActive,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
		SlowConsumerThreshold: time.Duration(req.SlowConsumerThreshold) * time.Millisecond,
		SlowConsumerMaxCount:  req.SlowConsumerMaxCount,
	}

	if req.RetryPolicy != nil {
		ep.RetryPolicy = *req.RetryPolicy
	} else {
		ep.RetryPolicy = model.RetryPolicy{
			Strategy:    model.RetryExponential,
			MaxRetries:  5,
			BaseDelayMs: 1000,
			MaxDelayMs:  60000,
			JitterMs:    500,
		}
	}

	if ep.SlowConsumerThreshold == 0 {
		ep.SlowConsumerThreshold = 5 * time.Second
	}
	if ep.SlowConsumerMaxCount == 0 {
		ep.SlowConsumerMaxCount = 5
	}

	if err := h.store.CreateEndpoint(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, ep)
}

func (h *Handler) ListEndpoints(c *gin.Context) {
	eps, err := h.store.ListEndpoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, eps)
}

func (h *Handler) GetEndpoint(c *gin.Context) {
	id := c.Param("id")
	ep, err := h.store.GetEndpoint(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}
	c.JSON(http.StatusOK, ep)
}

type UpdateEndpointRequest struct {
	Name                  string             `json:"name"`
	URL                   string             `json:"url"`
	EventTypes            []string           `json:"event_types"`
	RetryPolicy           *model.RetryPolicy `json:"retry_policy"`
	SlowConsumerThreshold int64              `json:"slow_consumer_threshold_ms"`
	SlowConsumerMaxCount  int                `json:"slow_consumer_max_count"`
}

func (h *Handler) UpdateEndpoint(c *gin.Context) {
	id := c.Param("id")
	ep, err := h.store.GetEndpoint(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}

	var req UpdateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		ep.Name = req.Name
	}
	if req.URL != "" {
		ep.URL = req.URL
	}
	if len(req.EventTypes) > 0 {
		ep.EventTypes = req.EventTypes
	}
	if req.RetryPolicy != nil {
		ep.RetryPolicy = *req.RetryPolicy
	}
	if req.SlowConsumerThreshold > 0 {
		ep.SlowConsumerThreshold = time.Duration(req.SlowConsumerThreshold) * time.Millisecond
	}
	if req.SlowConsumerMaxCount > 0 {
		ep.SlowConsumerMaxCount = req.SlowConsumerMaxCount
	}

	if err := h.store.UpdateEndpoint(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ep)
}

func (h *Handler) DeleteEndpoint(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteEndpoint(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type UpdateStatusRequest struct {
	Status model.EndpointStatus `json:"status" binding:"required"`
}

func (h *Handler) UpdateEndpointStatus(c *gin.Context) {
	id := c.Param("id")
	ep, err := h.store.GetEndpoint(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.Status {
	case model.EndpointActive, model.EndpointPaused, model.EndpointDisabled:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	ep.Status = req.Status
	if req.Status == model.EndpointActive {
		ep.SlowConsumerHitCount = 0
	}

	if err := h.store.UpdateEndpoint(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ep)
}

type AddAPIKeyRequest struct {
	KeyID  string `json:"key_id" binding:"required"`
	Secret string `json:"secret" binding:"required"`
}

func (h *Handler) AddAPIKey(c *gin.Context) {
	id := c.Param("id")
	ep, err := h.store.GetEndpoint(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}

	var req AddAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, k := range ep.APIKeys {
		if k.KeyID == req.KeyID {
			c.JSON(http.StatusConflict, gin.H{"error": "key_id already exists on this endpoint"})
			return
		}
	}

	allEps, err := h.store.ListEndpoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, other := range allEps {
		if other.ID == id {
			continue
		}
		for _, k := range other.APIKeys {
			if k.KeyID == req.KeyID {
				c.JSON(http.StatusConflict, gin.H{
					"error":       "key_id already in use by another endpoint",
					"endpoint_id": other.ID,
				})
				return
			}
		}
	}

	newKey := model.APIKeyPair{
		KeyID:     req.KeyID,
		Secret:    req.Secret,
		CreatedAt: time.Now().UTC(),
	}
	ep.APIKeys = append(ep.APIKeys, newKey)

	if err := h.store.UpdateEndpoint(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, ep)
}

func (h *Handler) DeleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	keyID := c.Param("keyId")

	ep, err := h.store.GetEndpoint(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
		return
	}

	if len(ep.APIKeys) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete last key"})
		return
	}

	newKeys := []model.APIKeyPair{}
	found := false
	for _, k := range ep.APIKeys {
		if k.KeyID != keyID {
			newKeys = append(newKeys, k)
		} else {
			found = true
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		return
	}

	ep.APIKeys = newKeys

	if err := h.store.UpdateEndpoint(ep); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ep)
}

func (h *Handler) GetEndpointHealth(c *gin.Context) {
	id := c.Param("id")
	health, err := h.healthChecker.GetEndpointHealth(id, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, health)
}

func (h *Handler) ListEndpointDeliveries(c *gin.Context) {
	id := c.Param("id")
	deliveries, err := h.store.ListDeliveriesByEndpoint(id, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deliveries)
}

func (h *Handler) GetEvent(c *gin.Context) {
	id := c.Param("id")
	event, err := h.store.GetEvent(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}
	c.JSON(http.StatusOK, event)
}

func (h *Handler) ListDeadLetters(c *gin.Context) {
	endpointID := c.Query("endpoint_id")
	onlyUnresolved := c.Query("only_unresolved") == "true"

	dls, err := h.store.ListDeadLetters(endpointID, onlyUnresolved, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dls)
}

func (h *Handler) GetDeadLetter(c *gin.Context) {
	id := c.Param("id")
	dl, err := h.store.GetDeadLetter(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dead letter not found"})
		return
	}
	c.JSON(http.StatusOK, dl)
}

func (h *Handler) RetryDeadLetter(c *gin.Context) {
	id := c.Param("id")
	if err := h.dispatcher.RetryDeadLetter(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "retried"})
}

type BatchRetryRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

func (h *Handler) BatchRetryDeadLetters(c *gin.Context) {
	var req BatchRetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make(map[string]string)
	for _, id := range req.IDs {
		err := h.dispatcher.RetryDeadLetter(id)
		if err != nil {
			results[id] = err.Error()
		} else {
			results[id] = "retried"
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

type ResolveRequest struct {
	ResolvedBy string `json:"resolved_by"`
}

func (h *Handler) ResolveDeadLetter(c *gin.Context) {
	id := c.Param("id")
	var req ResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.ResolvedBy = "admin"
	}

	dl, err := h.store.GetDeadLetter(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dead letter not found"})
		return
	}

	dl.Resolved = true
	dl.ResolvedAt = time.Now().UTC()
	dl.ResolvedBy = req.ResolvedBy

	if err := h.store.UpdateDeadLetter(dl); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dl)
}

func (h *Handler) DeleteDeadLetter(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteDeadLetter(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handler) SystemHealth(c *gin.Context) {
	eps, err := h.store.ListEndpoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dls, err := h.store.ListDeadLetters("", true, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"endpoints_count": len(eps),
		"dead_letters_count": len(dls),
		"timestamp":       time.Now().UTC(),
	})
}

func (h *Handler) AllEndpointsHealth(c *gin.Context) {
	healthList, err := h.healthChecker.GetAllEndpointsHealth(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, healthList)
}
