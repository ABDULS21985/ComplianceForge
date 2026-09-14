package queue

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Dispatcher maps versioned message types to handlers. Registration is
// explicit and duplicate registrations fail, preventing silent handler swaps.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]EnvelopeHandler
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]EnvelopeHandler)}
}

func (d *Dispatcher) Register(messageType string, handler EnvelopeHandler) error {
	if err := validateBrokerName(messageType, 255); err != nil {
		return fmt.Errorf("invalid message type: %w", err)
	}
	if handler == nil {
		return fmt.Errorf("handler for %s is required", messageType)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.handlers[messageType]; exists {
		return fmt.Errorf("handler for %s is already registered", messageType)
	}
	d.handlers[messageType] = handler
	return nil
}

func (d *Dispatcher) Handle(ctx context.Context, envelope Envelope) error {
	d.mu.RLock()
	handler := d.handlers[envelope.Type]
	d.mu.RUnlock()
	if handler == nil {
		return Permanent(fmt.Errorf("no handler registered for message type %s", envelope.Type))
	}
	return handler(ctx, envelope)
}

func (d *Dispatcher) Types() []string {
	d.mu.RLock()
	types := make([]string, 0, len(d.handlers))
	for messageType := range d.handlers {
		types = append(types, messageType)
	}
	d.mu.RUnlock()
	sort.Strings(types)
	return types
}
