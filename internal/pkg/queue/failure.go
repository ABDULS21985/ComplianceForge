package queue

import (
	"errors"
	"fmt"
)

type FailureRoute uint8

const (
	FailureRetry FailureRoute = iota
	FailureDeadLetter
	FailureQuarantine
)

// PermanentError marks a syntactically valid job that cannot succeed when
// retried, such as an unknown job type or invalid business parameters.
type PermanentError struct {
	err error
}

func (e *PermanentError) Error() string { return e.err.Error() }
func (e *PermanentError) Unwrap() error { return e.err }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{err: err}
}

func failureRoute(attempt, maxAttempts int, handlerErr error) FailureRoute {
	var permanent *PermanentError
	if errors.As(handlerErr, &permanent) || attempt >= maxAttempts {
		return FailureDeadLetter
	}
	return FailureRetry
}

func failureReason(err error) string {
	if err == nil {
		return "unknown failure"
	}
	const maxReasonBytes = 512
	return boundedAMQPString(err.Error(), maxReasonBytes)
}

var (
	ErrClosed                  = errors.New("queue service is closed")
	ErrPublishNack             = errors.New("RabbitMQ negatively acknowledged the publish")
	ErrReturnStream            = errors.New("RabbitMQ return stream closed before publish confirmation")
	ErrDeduplicationInProgress = errors.New("message attempt is already being processed")
	ErrDeduplicationLeaseLost  = errors.New("message processing lease was lost")
	ErrDeduplicationConflict   = errors.New("message idempotency key conflicts with a different envelope")
)

type UnroutableError struct {
	Exchange   string
	RoutingKey string
	ReplyCode  uint16
	ReplyText  string
}

func (e *UnroutableError) Error() string {
	return fmt.Sprintf("message was unroutable on exchange %s with key %s: %d %s", e.Exchange, e.RoutingKey, e.ReplyCode, e.ReplyText)
}
