package health

import (
	"sort"

	"webhook-service/internal/model"
	"webhook-service/internal/store"
)

type Checker struct {
	store *store.Store
}

func NewChecker(store *store.Store) *Checker {
	return &Checker{store: store}
}

func (hc *Checker) GetEndpointHealth(endpointID string, recentDeliveries int) (*model.EndpointHealth, error) {
	deliveries, err := hc.store.ListDeliveriesByEndpoint(endpointID, recentDeliveries)
	if err != nil {
		return nil, err
	}

	health := &model.EndpointHealth{
		EndpointID: endpointID,
	}

	if len(deliveries) == 0 {
		return health, nil
	}

	health.TotalDeliveries = len(deliveries)

	var latencies []float64
	successCount := 0
	failureCount := 0
	lastErrorCode := 0

	for _, d := range deliveries {
		if d.Status == model.DeliverySuccess {
			successCount++
		} else {
			failureCount++
			if d.StatusCode > 0 {
				lastErrorCode = d.StatusCode
			}
		}
		if d.DurationMs > 0 {
			latencies = append(latencies, float64(d.DurationMs))
		}
	}

	health.SuccessCount = successCount
	health.FailureCount = failureCount
	health.LastErrorCode = lastErrorCode

	if len(deliveries) > 0 {
		health.SuccessRate = float64(successCount) / float64(len(deliveries)) * 100
	}

	if len(latencies) > 0 {
		var sum float64
		for _, l := range latencies {
			sum += l
		}
		health.AvgLatencyMs = sum / float64(len(latencies))

		sort.Float64s(latencies)
		p95Index := int(float64(len(latencies)) * 0.95)
		if p95Index >= len(latencies) {
			p95Index = len(latencies) - 1
		}
		health.P95LatencyMs = latencies[p95Index]
	}

	return health, nil
}

func (hc *Checker) GetAllEndpointsHealth(recentDeliveries int) ([]*model.EndpointHealth, error) {
	endpoints, err := hc.store.ListEndpoints()
	if err != nil {
		return nil, err
	}

	var healthList []*model.EndpointHealth
	for _, ep := range endpoints {
		h, err := hc.GetEndpointHealth(ep.ID, recentDeliveries)
		if err != nil {
			continue
		}
		healthList = append(healthList, h)
	}

	return healthList, nil
}
