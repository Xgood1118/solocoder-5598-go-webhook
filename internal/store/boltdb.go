package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"webhook-service/internal/model"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketEndpoints   = "endpoints"
	bucketEvents      = "events"
	bucketDeliveries  = "deliveries"
	bucketDeadLetters = "dead_letters"
	bucketEventIDs    = "event_ids"
)

type Store struct {
	db *bolt.DB
}

func NewStore(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open boltdb: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		buckets := []string{bucketEndpoints, bucketEvents, bucketDeliveries, bucketDeadLetters, bucketEventIDs}
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return fmt.Errorf("create bucket %s: %w", b, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) IsDuplicateEvent(eventID string) (bool, error) {
	var exists bool
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEventIDs))
		exists = b.Get([]byte(eventID)) != nil
		return nil
	})
	return exists, err
}

func (s *Store) MarkEventReceived(eventID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEventIDs))
		return b.Put([]byte(eventID), []byte(time.Now().UTC().Format(time.RFC3339)))
	})
}

func (s *Store) CreateEvent(event *model.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEvents))
		return b.Put([]byte(event.ID), data)
	})
}

func (s *Store) GetEvent(id string) (*model.Event, error) {
	var event model.Event
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEvents))
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("event not found: %s", id)
		}
		return json.Unmarshal(data, &event)
	})
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *Store) UpdateEvent(event *model.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEvents))
		return b.Put([]byte(event.ID), data)
	})
}

func (s *Store) CreateEndpoint(ep *model.Endpoint) error {
	data, err := json.Marshal(ep)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		return b.Put([]byte(ep.ID), data)
	})
}

func (s *Store) GetEndpoint(id string) (*model.Endpoint, error) {
	var ep model.Endpoint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("endpoint not found: %s", id)
		}
		return json.Unmarshal(data, &ep)
	})
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func (s *Store) UpdateEndpoint(ep *model.Endpoint) error {
	ep.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(ep)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		return b.Put([]byte(ep.ID), data)
	})
}

func (s *Store) DeleteEndpoint(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListEndpoints() ([]*model.Endpoint, error) {
	var eps []*model.Endpoint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		return b.ForEach(func(k, v []byte) error {
			var ep model.Endpoint
			if err := json.Unmarshal(v, &ep); err != nil {
				return err
			}
			eps = append(eps, &ep)
			return nil
		})
	})
	return eps, err
}

func (s *Store) ListActiveEndpointsByEventType(eventType string) ([]*model.Endpoint, error) {
	var eps []*model.Endpoint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketEndpoints))
		return b.ForEach(func(k, v []byte) error {
			var ep model.Endpoint
			if err := json.Unmarshal(v, &ep); err != nil {
				return err
			}
			if ep.Status != model.EndpointActive {
				return nil
			}
			for _, et := range ep.EventTypes {
				if et == eventType || et == "*" {
					eps = append(eps, &ep)
					break
				}
			}
			return nil
		})
	})
	return eps, err
}

func (s *Store) CreateDelivery(d *model.Delivery) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeliveries))
		return b.Put([]byte(d.ID), data)
	})
}

func (s *Store) GetDelivery(id string) (*model.Delivery, error) {
	var d model.Delivery
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeliveries))
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("delivery not found: %s", id)
		}
		return json.Unmarshal(data, &d)
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) UpdateDelivery(d *model.Delivery) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeliveries))
		return b.Put([]byte(d.ID), data)
	})
}

func (s *Store) ListDeliveriesByEndpoint(endpointID string, limit int) ([]*model.Delivery, error) {
	var deliveries []*model.Delivery
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeliveries))
		return b.ForEach(func(k, v []byte) error {
			var d model.Delivery
			if err := json.Unmarshal(v, &d); err != nil {
				return err
			}
			if d.EndpointID == endpointID {
				deliveries = append(deliveries, &d)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(deliveries, func(i, j int) bool {
		return deliveries[i].CreatedAt.After(deliveries[j].CreatedAt)
	})
	if limit > 0 && len(deliveries) > limit {
		deliveries = deliveries[:limit]
	}
	return deliveries, nil
}

func (s *Store) CreateDeadLetter(dl *model.DeadLetter) error {
	data, err := json.Marshal(dl)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeadLetters))
		return b.Put([]byte(dl.ID), data)
	})
}

func (s *Store) GetDeadLetter(id string) (*model.DeadLetter, error) {
	var dl model.DeadLetter
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeadLetters))
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("dead letter not found: %s", id)
		}
		return json.Unmarshal(data, &dl)
	})
	if err != nil {
		return nil, err
	}
	return &dl, nil
}

func (s *Store) UpdateDeadLetter(dl *model.DeadLetter) error {
	data, err := json.Marshal(dl)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeadLetters))
		return b.Put([]byte(dl.ID), data)
	})
}

func (s *Store) DeleteDeadLetter(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeadLetters))
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListDeadLetters(endpointID string, onlyUnresolved bool, limit int) ([]*model.DeadLetter, error) {
	var dls []*model.DeadLetter
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeadLetters))
		return b.ForEach(func(k, v []byte) error {
			var dl model.DeadLetter
			if err := json.Unmarshal(v, &dl); err != nil {
				return err
			}
			if endpointID != "" && dl.EndpointID != endpointID {
				return nil
			}
			if onlyUnresolved && dl.Resolved {
				return nil
			}
			dls = append(dls, &dl)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(dls, func(i, j int) bool {
		return dls[i].CreatedAt.After(dls[j].CreatedAt)
	})
	if limit > 0 && len(dls) > limit {
		dls = dls[:limit]
	}
	return dls, nil
}
