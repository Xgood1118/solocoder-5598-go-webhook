package retry

import (
	"math"
	"math/rand"
	"time"

	"webhook-service/internal/model"
)

func CalculateNextDelay(policy model.RetryPolicy, attempt int) time.Duration {
	var baseDelay int
	switch policy.Strategy {
	case model.RetryImmediate:
		baseDelay = 0
	case model.RetryFixed:
		baseDelay = policy.BaseDelayMs
	case model.RetryExponential:
		if attempt <= 0 {
			attempt = 1
		}
		exp := math.Pow(2, float64(attempt-1))
		baseDelay = int(float64(policy.BaseDelayMs) * exp)
		if policy.MaxDelayMs > 0 && baseDelay > policy.MaxDelayMs {
			baseDelay = policy.MaxDelayMs
		}
	default:
		baseDelay = policy.BaseDelayMs
	}

	jitter := 0
	if policy.JitterMs > 0 {
		jitter = rand.Intn(policy.JitterMs) - policy.JitterMs/2
	}

	total := baseDelay + jitter
	if total < 0 {
		total = 0
	}

	return time.Duration(total) * time.Millisecond
}

func ShouldRetry(policy model.RetryPolicy, attempt int) bool {
	return attempt <= policy.MaxRetries
}
