package model

import (
	"time"
)

type EndpointStatus string

const (
	EndpointActive   EndpointStatus = "active"
	EndpointPaused   EndpointStatus = "paused"
	EndpointDisabled EndpointStatus = "disabled"
)

type RetryStrategyType string

const (
	RetryImmediate   RetryStrategyType = "immediate"
	RetryFixed       RetryStrategyType = "fixed"
	RetryExponential RetryStrategyType = "exponential"
)

type APIKeyPair struct {
	KeyID     string    `json:"key_id"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
}

type Endpoint struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	EventTypes  []string       `json:"event_types"`
	APIKeys     []APIKeyPair   `json:"api_keys"`
	Status      EndpointStatus `json:"status"`
	RetryPolicy RetryPolicy    `json:"retry_policy"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`

	SlowConsumerThreshold time.Duration `json:"slow_consumer_threshold"`
	SlowConsumerMaxCount  int           `json:"slow_consumer_max_count"`
	SlowConsumerHitCount  int           `json:"slow_consumer_hit_count"`
}

type RetryPolicy struct {
	Strategy    RetryStrategyType `json:"strategy"`
	MaxRetries  int               `json:"max_retries"`
	BaseDelayMs int               `json:"base_delay_ms"`
	MaxDelayMs  int               `json:"max_delay_ms"`
	JitterMs    int               `json:"jitter_ms"`
}

type Event struct {
	ID            string    `json:"id"`
	EventType     string    `json:"event_type"`
	Payload       []byte    `json:"payload"`
	SourceIP      string    `json:"source_ip"`
	Signature     string    `json:"signature"`
	KeyID         string    `json:"key_id"`
	Timestamp     int64     `json:"timestamp"`
	SignatureOK   bool      `json:"signature_ok"`
	ReceivedAt    time.Time `json:"received_at"`
	Dispatched    bool      `json:"dispatched"`
	DispatchedAt  time.Time `json:"dispatched_at,omitempty"`
}

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliverySuccess   DeliveryStatus = "success"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryRetrying  DeliveryStatus = "retrying"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
)

type Delivery struct {
	ID             string         `json:"id"`
	EventID        string         `json:"event_id"`
	EndpointID     string         `json:"endpoint_id"`
	Status         DeliveryStatus `json:"status"`
	Attempt        int            `json:"attempt"`
	MaxRetries     int            `json:"max_retries"`
	StatusCode     int            `json:"status_code,omitempty"`
	ResponseBody   string         `json:"response_body,omitempty"`
	DurationMs     int64          `json:"duration_ms"`
	LastAttemptAt  time.Time      `json:"last_attempt_at,omitempty"`
	NextAttemptAt  time.Time      `json:"next_attempt_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	LastError      string         `json:"last_error,omitempty"`
}

type DeadLetter struct {
	ID            string    `json:"id"`
	DeliveryID    string    `json:"delivery_id"`
	EventID       string    `json:"event_id"`
	EndpointID    string    `json:"endpoint_id"`
	EventType     string    `json:"event_type"`
	Payload       []byte    `json:"payload"`
	LastError     string    `json:"last_error"`
	LastStatusCode int       `json:"last_status_code"`
	RetryCount    int       `json:"retry_count"`
	CreatedAt     time.Time `json:"created_at"`
	Resolved      bool      `json:"resolved"`
	ResolvedAt    time.Time `json:"resolved_at,omitempty"`
	ResolvedBy    string    `json:"resolved_by,omitempty"`
}

type EndpointHealth struct {
	EndpointID    string  `json:"endpoint_id"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	LastErrorCode int     `json:"last_error_code,omitempty"`
	TotalDeliveries int   `json:"total_deliveries"`
	SuccessCount  int     `json:"success_count"`
	FailureCount  int     `json:"failure_count"`
}
