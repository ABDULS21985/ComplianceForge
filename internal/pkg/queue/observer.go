package queue

import "time"

// Observer receives only bounded operation/outcome values. Implementations
// must never add message, tenant, queue, or correlation identifiers as metric
// labels.
type Observer interface {
	ObserveQueueOperation(operation, outcome string, duration time.Duration)
	ObserveQueueMessageAge(age time.Duration)
	ObserveQueueReconnect(component string)
}

type noopObserver struct{}

func (noopObserver) ObserveQueueOperation(string, string, time.Duration) {}
func (noopObserver) ObserveQueueMessageAge(time.Duration)                {}
func (noopObserver) ObserveQueueReconnect(string)                        {}
