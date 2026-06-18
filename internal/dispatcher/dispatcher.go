package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"webhook-service/internal/model"
	"webhook-service/internal/retry"
	"webhook-service/internal/store"
	"webhook-service/internal/webhook"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Dispatcher struct {
	store       *store.Store
	workerCount int
	eventQueue  chan *model.Event
	retryQueue  chan *model.Delivery
	client      *http.Client
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewDispatcher(store *store.Store, workerCount int, queueSize int) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		store:       store,
		workerCount: workerCount,
		eventQueue:  make(chan *model.Event, queueSize),
		retryQueue:  make(chan *model.Delivery, queueSize),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (d *Dispatcher) Start() {
	for i := 0; i < d.workerCount; i++ {
		d.wg.Add(1)
		go d.worker(i)
	}
	log.Info().Int("workers", d.workerCount).Msg("dispatcher started")
}

func (d *Dispatcher) Stop() {
	d.cancel()
	close(d.eventQueue)
	close(d.retryQueue)
	d.wg.Wait()
	log.Info().Msg("dispatcher stopped")
}

func (d *Dispatcher) Dispatch(event *model.Event) {
	select {
	case d.eventQueue <- event:
	default:
		log.Warn().Str("event_id", event.ID).Msg("event queue full, dropping event")
	}
}

func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()
	log.Debug().Int("worker_id", id).Msg("worker started")

	for {
		select {
		case <-d.ctx.Done():
			return
		case event, ok := <-d.eventQueue:
			if !ok {
				return
			}
			d.handleEvent(event)
		case delivery, ok := <-d.retryQueue:
			if !ok {
				return
			}
			d.handleRetry(delivery)
		}
	}
}

func (d *Dispatcher) handleEvent(event *model.Event) {
	endpoints, err := d.store.ListActiveEndpointsByEventType(event.EventType)
	if err != nil {
		log.Error().Err(err).Str("event_id", event.ID).Msg("failed to list endpoints")
		return
	}

	if len(endpoints) == 0 {
		log.Debug().Str("event_id", event.ID).Str("event_type", event.EventType).Msg("no active endpoints for event type")
		return
	}

	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(ep *model.Endpoint) {
			defer wg.Done()
			d.createAndDeliver(event, ep)
		}(ep)
	}
	wg.Wait()

	event.Dispatched = true
	event.DispatchedAt = time.Now().UTC()
	if err := d.store.UpdateEvent(event); err != nil {
		log.Error().Err(err).Str("event_id", event.ID).Msg("failed to update event")
	}
}

func (d *Dispatcher) createAndDeliver(event *model.Event, ep *model.Endpoint) {
	delivery := &model.Delivery{
		ID:         uuid.New().String(),
		EventID:    event.ID,
		EndpointID: ep.ID,
		Status:     model.DeliveryPending,
		Attempt:    0,
		MaxRetries: ep.RetryPolicy.MaxRetries,
		CreatedAt:  time.Now().UTC(),
	}

	if err := d.store.CreateDelivery(delivery); err != nil {
		log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to create delivery")
		return
	}

	d.deliver(delivery, event, ep)
}

func (d *Dispatcher) deliver(delivery *model.Delivery, event *model.Event, ep *model.Endpoint) {
	delivery.Attempt++
	delivery.Status = model.DeliveryRetrying
	if delivery.Attempt > 1 {
		delivery.Status = model.DeliveryRetrying
	}
	start := time.Now()

	signature := ""
	timestamp := time.Now().Unix()
	if len(ep.APIKeys) > 0 {
		activeKey := ep.APIKeys[len(ep.APIKeys)-1]
		signature = webhook.GenerateHMAC(event.Payload, timestamp, activeKey.Secret)
	}

	req, err := http.NewRequest("POST", ep.URL, bytes.NewReader(event.Payload))
	if err != nil {
		d.handleDeliveryError(delivery, event, ep, fmt.Sprintf("create request error: %v", err), 0)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Event-Type", event.EventType)
	if signature != "" && len(ep.APIKeys) > 0 {
		req.Header.Set("X-Signature", signature)
		req.Header.Set("X-Key-ID", ep.APIKeys[len(ep.APIKeys)-1].KeyID)
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	}

	resp, err := d.client.Do(req)
	duration := time.Since(start)
	delivery.DurationMs = duration.Milliseconds()

	if err != nil {
		d.handleDeliveryError(delivery, event, ep, fmt.Sprintf("request error: %v", err), 0)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	delivery.StatusCode = resp.StatusCode
	delivery.ResponseBody = string(body)
	delivery.LastAttemptAt = time.Now().UTC()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Status = model.DeliverySuccess
		delivery.LastError = ""
		if err := d.store.UpdateDelivery(delivery); err != nil {
			log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to update delivery")
		}
		d.checkSlowConsumerAndReset(ep, duration)
		return
	}

	d.handleDeliveryError(delivery, event, ep, fmt.Sprintf("status code %d", resp.StatusCode), resp.StatusCode)
}

func (d *Dispatcher) handleDeliveryError(delivery *model.Delivery, event *model.Event, ep *model.Endpoint, errMsg string, statusCode int) {
	delivery.Status = model.DeliveryFailed
	delivery.LastError = errMsg
	delivery.LastAttemptAt = time.Now().UTC()
	if statusCode > 0 {
		delivery.StatusCode = statusCode
	}

	log.Warn().
		Str("delivery_id", delivery.ID).
		Str("endpoint_id", ep.ID).
		Int("attempt", delivery.Attempt).
		Int("max_retries", delivery.MaxRetries).
		Str("error", errMsg).
		Msg("delivery failed")

	if delivery.Attempt >= delivery.MaxRetries {
		d.moveToDeadLetter(delivery, event)
		return
	}

	nextDelay := retry.CalculateNextDelay(ep.RetryPolicy, delivery.Attempt)
	delivery.NextAttemptAt = time.Now().UTC().Add(nextDelay)
	delivery.Status = model.DeliveryRetrying

	if err := d.store.UpdateDelivery(delivery); err != nil {
		log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to update delivery")
	}

	d.scheduleRetry(delivery, nextDelay)
}

func (d *Dispatcher) scheduleRetry(delivery *model.Delivery, delay time.Duration) {
	go func() {
		select {
		case <-d.ctx.Done():
			return
		case <-time.After(delay):
			select {
			case d.retryQueue <- delivery:
			default:
				log.Warn().Str("delivery_id", delivery.ID).Msg("retry queue full, will retry later")
			}
		}
	}()
}

func (d *Dispatcher) handleRetry(delivery *model.Delivery) {
	event, err := d.store.GetEvent(delivery.EventID)
	if err != nil {
		log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to get event for retry")
		return
	}

	ep, err := d.store.GetEndpoint(delivery.EndpointID)
	if err != nil {
		log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to get endpoint for retry")
		return
	}

	if ep.Status != model.EndpointActive {
		log.Info().Str("delivery_id", delivery.ID).Str("status", string(ep.Status)).Msg("endpoint not active, skipping retry")
		return
	}

	d.deliver(delivery, event, ep)
}

func (d *Dispatcher) moveToDeadLetter(delivery *model.Delivery, event *model.Event) {
	delivery.Status = model.DeliveryDeadLetter
	if err := d.store.UpdateDelivery(delivery); err != nil {
		log.Error().Err(err).Str("delivery_id", delivery.ID).Msg("failed to update delivery to dead letter")
	}

	dl := &model.DeadLetter{
		ID:             uuid.New().String(),
		DeliveryID:     delivery.ID,
		EventID:        event.ID,
		EndpointID:     delivery.EndpointID,
		EventType:      event.EventType,
		Payload:        event.Payload,
		LastError:      delivery.LastError,
		LastStatusCode: delivery.StatusCode,
		RetryCount:     delivery.Attempt,
		CreatedAt:      time.Now().UTC(),
		Resolved:       false,
	}

	if err := d.store.CreateDeadLetter(dl); err != nil {
		log.Error().Err(err).Str("dead_letter_id", dl.ID).Msg("failed to create dead letter")
	}

	log.Warn().
		Str("dead_letter_id", dl.ID).
		Str("delivery_id", delivery.ID).
		Str("endpoint_id", delivery.EndpointID).
		Int("retries", delivery.Attempt).
		Msg("moved to dead letter")
}

func (d *Dispatcher) RetryDeadLetter(dlID string) error {
	dl, err := d.store.GetDeadLetter(dlID)
	if err != nil {
		return fmt.Errorf("dead letter not found: %w", err)
	}

	ep, err := d.store.GetEndpoint(dl.EndpointID)
	if err != nil {
		return fmt.Errorf("endpoint not found: %w", err)
	}

	if ep.Status != model.EndpointActive {
		return fmt.Errorf("endpoint is not active: %s", ep.Status)
	}

	event, err := d.store.GetEvent(dl.EventID)
	if err != nil {
		event = &model.Event{
			ID:        dl.EventID,
			EventType: dl.EventType,
			Payload:   dl.Payload,
		}
	}

	newDelivery := &model.Delivery{
		ID:         uuid.New().String(),
		EventID:    event.ID,
		EndpointID: ep.ID,
		Status:     model.DeliveryPending,
		Attempt:    0,
		MaxRetries: ep.RetryPolicy.MaxRetries,
		CreatedAt:  time.Now().UTC(),
	}

	if err := d.store.CreateDelivery(newDelivery); err != nil {
		return fmt.Errorf("create delivery: %w", err)
	}

	dl.Resolved = true
	dl.ResolvedAt = time.Now().UTC()
	dl.ResolvedBy = "manual_retry"
	if err := d.store.UpdateDeadLetter(dl); err != nil {
		log.Error().Err(err).Str("dead_letter_id", dl.ID).Msg("failed to update dead letter")
	}

	go d.createAndDeliver(event, ep)

	return nil
}

func (d *Dispatcher) checkSlowConsumerAndReset(ep *model.Endpoint, duration time.Duration) {
	if ep.SlowConsumerThreshold <= 0 || ep.SlowConsumerMaxCount <= 0 {
		return
	}

	fresh, err := d.store.GetEndpoint(ep.ID)
	if err != nil {
		log.Error().Err(err).Str("endpoint_id", ep.ID).Msg("failed to read endpoint for slow consumer check")
		return
	}

	changed := false
	if duration > fresh.SlowConsumerThreshold {
		fresh.SlowConsumerHitCount++
		changed = true
		if fresh.SlowConsumerHitCount >= fresh.SlowConsumerMaxCount {
			fresh.Status = model.EndpointPaused
			fresh.SlowConsumerHitCount = 0
			log.Warn().
				Str("endpoint_id", fresh.ID).
				Int("hit_count", fresh.SlowConsumerMaxCount).
				Dur("threshold", fresh.SlowConsumerThreshold).
				Msg("slow consumer detected, pausing endpoint")
		}
	} else {
		if fresh.SlowConsumerHitCount > 0 {
			fresh.SlowConsumerHitCount = 0
			changed = true
		}
	}

	if changed {
		if err := d.store.UpdateEndpoint(fresh); err != nil {
			log.Error().Err(err).Str("endpoint_id", fresh.ID).Msg("failed to update endpoint slow consumer status")
		}
	}
}
