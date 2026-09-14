package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFailureRouting(t *testing.T) {
	retryable := errors.New("temporary database timeout")
	if got := failureRoute(1, 5, retryable); got != FailureRetry {
		t.Fatalf("first transient failure routed to %v", got)
	}
	if got := failureRoute(5, 5, retryable); got != FailureDeadLetter {
		t.Fatalf("exhausted failure routed to %v", got)
	}
	if got := failureRoute(1, 5, Permanent(errors.New("invalid job"))); got != FailureDeadLetter {
		t.Fatalf("permanent failure routed to %v", got)
	}
}

func TestFailureReasonIsBounded(t *testing.T) {
	reason := failureReason(errors.New(strings.Repeat("x", 1000)))
	if len(reason) != 512 {
		t.Fatalf("failure reason length = %d", len(reason))
	}
}

func TestInvokeHandlerConvertsPanicToError(t *testing.T) {
	err := invokeHandler(context.Background(), func(context.Context, Envelope) error {
		panic("boom")
	}, Envelope{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("panic result = %v", err)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	if got := nextBackoff(2, 10); got != 4 {
		t.Fatalf("nextBackoff = %s", got)
	}
	if got := nextBackoff(8, 10); got != 10 {
		t.Fatalf("capped nextBackoff = %s", got)
	}
}

func TestMetadataWithFailureStaysWithinEnvelopeLimit(t *testing.T) {
	metadata := make(map[string]string, maxEnvelopeMetadataEntries)
	for index := 0; index < maxEnvelopeMetadataEntries; index++ {
		metadata[fmt.Sprintf("key-%d", index)] = "value"
	}
	withFailure := metadataWithFailure(metadata, "last_failure", "temporary")
	if len(withFailure) > maxEnvelopeMetadataEntries {
		t.Fatalf("failure metadata contains %d entries", len(withFailure))
	}

	metadata["last_failure"] = "temporary"
	delete(metadata, "key-0")
	terminal := metadataWithFailure(metadata, "terminal_failure", "exhausted")
	if len(terminal) != maxEnvelopeMetadataEntries || terminal["terminal_failure"] != "exhausted" {
		t.Fatalf("terminal metadata = %+v", terminal)
	}
	if _, exists := terminal["last_failure"]; exists {
		t.Fatal("stale retry failure remained on terminal envelope")
	}
}
