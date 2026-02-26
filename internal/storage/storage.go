package storage

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// TrackedOrderStatus represents the local tracking status for an order.
type TrackedOrderStatus string

const (
	StatusOfferPending      TrackedOrderStatus = "offer_pending"
	StatusAssigned          TrackedOrderStatus = "assigned"
	StatusAssignedNotified  TrackedOrderStatus = "assigned_notified"
	StatusClosed            TrackedOrderStatus = "closed"
)

// TrackedOrder holds minimal info to track order state changes.
type TrackedOrder struct {
	OrderID         string             `json:"orderId"`
	LastKnownStatus string             `json:"lastKnownStatus"`
	TrackingStatus  TrackedOrderStatus `json:"trackingStatus"`
	LastStatusCheck time.Time          `json:"lastStatusCheck"`
}

type state struct {
	SeenOrderIDs  map[string]bool        `json:"seenOrderIds"`
	TrackedOrders map[string]TrackedOrder `json:"trackedOrders"`
}

// JSONStorage is a simple JSON-file-based implementation for tracking orders.
type JSONStorage struct {
	path  string
	mu    sync.Mutex
	state state
}

func NewJSONStorage(path string) (*JSONStorage, error) {
	s := &JSONStorage{
		path: path,
		state: state{
			SeenOrderIDs:  make(map[string]bool),
			TrackedOrders: make(map[string]TrackedOrder),
		},
	}

	if err := s.load(); err != nil {
		// If file does not exist we start with empty state.
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return s, nil
}

func (s *JSONStorage) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var st state
	if err := dec.Decode(&st); err != nil {
		return err
	}

	if st.SeenOrderIDs == nil {
		st.SeenOrderIDs = make(map[string]bool)
	}
	if st.TrackedOrders == nil {
		st.TrackedOrders = make(map[string]TrackedOrder)
	}

	s.state = st
	return nil
}

func (s *JSONStorage) persist() error {
	tmpPath := s.path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&s.state); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// MarkOrderSeen marks an order as seen to avoid duplicate processing.
func (s *JSONStorage) MarkOrderSeen(orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.SeenOrderIDs[orderID] {
		return nil
	}
	s.state.SeenOrderIDs[orderID] = true
	return s.persist()
}

// IsOrderSeen returns true if we already saw this order before.
func (s *JSONStorage) IsOrderSeen(orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.SeenOrderIDs[orderID]
}

// TrackOrder sets or updates tracked order info.
func (s *JSONStorage) TrackOrder(orderID string, lastKnownStatus string, trackingStatus TrackedOrderStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.TrackedOrders[orderID] = TrackedOrder{
		OrderID:         orderID,
		LastKnownStatus: lastKnownStatus,
		TrackingStatus:  trackingStatus,
		LastStatusCheck: time.Now(),
	}
	return s.persist()
}

// UpdateTrackedOrderStatus updates tracking and last known status.
func (s *JSONStorage) UpdateTrackedOrderStatus(orderID string, lastKnownStatus string, trackingStatus TrackedOrderStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tr, ok := s.state.TrackedOrders[orderID]
	if !ok {
		tr = TrackedOrder{OrderID: orderID}
	}
	tr.LastKnownStatus = lastKnownStatus
	tr.TrackingStatus = trackingStatus
	tr.LastStatusCheck = time.Now()
	s.state.TrackedOrders[orderID] = tr
	return s.persist()
}

// ListTrackedOrdersByStatus returns all tracked orders with the given tracking status.
func (s *JSONStorage) ListTrackedOrdersByStatus(status TrackedOrderStatus) []TrackedOrder {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res []TrackedOrder
	for _, tr := range s.state.TrackedOrders {
		if tr.TrackingStatus == status {
			res = append(res, tr)
		}
	}
	return res
}

