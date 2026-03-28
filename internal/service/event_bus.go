package service

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Event represents a domain event published on the EventBus.
type Event struct {
	Type       string                 `json:"type"`        // e.g., "incident.created"
	Severity   string                 `json:"severity"`    // critical, high, medium, low
	OrgID      string                 `json:"org_id"`
	EntityType string                 `json:"entity_type"` // incident, control, policy, etc.
	EntityID   string                 `json:"entity_id"`
	EntityRef  string                 `json:"entity_ref"` // e.g., INC-0001
	Data       map[string]interface{} `json:"data"`       // template variables / payload
	Timestamp  time.Time              `json:"timestamp"`
}

// EventBus is a channel-based in-process publish/subscribe event bus.
// It is safe for concurrent use.
type EventBus struct {
	subscribers map[string][]chan Event
	mu          sync.RWMutex
}

// NewEventBus creates a new EventBus with an empty subscribers map.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

// Subscribe registers a channel for a given event type and returns a read-only
// channel. Use "*" to subscribe to all event types.
func (eb *EventBus) Subscribe(eventType string) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, 64)
	eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
	return ch
}

// Publish sends an event to all subscribers matching the event type and wildcard
// "*" subscribers. Events are dropped if the subscriber channel buffer is full.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Deliver to exact-match subscribers.
	for _, ch := range eb.subscribers[event.Type] {
		select {
		case ch <- event:
		default:
			log.Warn().
				Str("event_type", event.Type).
				Msg("subscriber channel full, dropping event")
		}
	}

	// Deliver to wildcard subscribers.
	if event.Type != "*" {
		for _, ch := range eb.subscribers["*"] {
			select {
			case ch <- event:
			default:
				log.Warn().
					Str("event_type", event.Type).
					Msg("wildcard subscriber channel full, dropping event")
			}
		}
	}
}

// Close closes all subscriber channels and clears the subscriber map.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType, channels := range eb.subscribers {
		for _, ch := range channels {
			close(ch)
		}
		delete(eb.subscribers, eventType)
	}
}
