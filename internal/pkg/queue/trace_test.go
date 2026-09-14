package queue

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestEnvelopeTraceContextRoundTripDoesNotMutateCallerMetadata(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	ctx, span := otel.Tracer("test").Start(context.Background(), "producer")
	original := map[string]string{"source": "test"}
	envelope, err := NewEnvelope("test.trace", "", map[string]string{"ok": "yes"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	envelope.Metadata = original
	injected := InjectTraceContext(ctx, envelope)
	span.End()

	if _, changed := original["traceparent"]; changed {
		t.Fatal("InjectTraceContext mutated caller-owned metadata")
	}
	if injected.Metadata["traceparent"] == "" || injected.Metadata["source"] != "test" {
		t.Fatalf("injected metadata = %#v", injected.Metadata)
	}
	extracted := ExtractTraceContext(context.Background(), injected)
	if got, want := trace.SpanContextFromContext(extracted).TraceID(), trace.SpanContextFromContext(ctx).TraceID(); got != want {
		t.Fatalf("extracted trace ID = %s, want %s", got, want)
	}
}

func TestQueueObserverRecordsValidationFailure(t *testing.T) {
	service, err := NewRabbitMQServiceWithConfig(DefaultConfig("amqp://guest:guest@localhost:5672/"), nil)
	if err != nil {
		t.Fatalf("NewRabbitMQServiceWithConfig() error = %v", err)
	}
	observer := &recordingQueueObserver{}
	service.SetObserver(observer)
	if err := service.PublishEnvelope(context.Background(), "invalid queue name", Envelope{}); err == nil {
		t.Fatal("PublishEnvelope() accepted invalid queue name")
	}
	if observer.operation != "publish" || observer.outcome != "error" || observer.duration < 0 {
		t.Fatalf("observer record = operation %q outcome %q duration %s", observer.operation, observer.outcome, observer.duration)
	}
}

type recordingQueueObserver struct {
	operation string
	outcome   string
	duration  time.Duration
}

func (o *recordingQueueObserver) ObserveQueueOperation(operation, outcome string, duration time.Duration) {
	o.operation, o.outcome, o.duration = operation, outcome, duration
}

func (*recordingQueueObserver) ObserveQueueMessageAge(time.Duration) {}
func (*recordingQueueObserver) ObserveQueueReconnect(string)         {}

var _ Observer = (*recordingQueueObserver)(nil)
