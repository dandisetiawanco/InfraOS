package sse

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	OrgID     string    `json:"organization_id"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

func (b *Broker) Subscribe(orgID string) (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	if b.subscribers[orgID] == nil {
		b.subscribers[orgID] = make(map[chan Event]struct{})
	}
	b.subscribers[orgID][ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if subs, ok := b.subscribers[orgID]; ok {
			if _, exists := subs[ch]; exists {
				delete(subs, ch)
				close(ch)
			}
			if len(subs) == 0 {
				delete(b.subscribers, orgID)
			}
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

func (b *Broker) Publish(orgID, eventType string, payload any) {
	ev := Event{
		Type:      eventType,
		OrgID:     orgID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
	b.mu.RLock()
	subs := b.subscribers[orgID]
	for ch := range subs {
		select {
		case ch <- ev:
		default:
			// Drop event for slow clients so producers never stall.
		}
	}
	b.mu.RUnlock()
}

func (e Event) EncodeJSON() ([]byte, error) {
	return json.Marshal(e)
}
