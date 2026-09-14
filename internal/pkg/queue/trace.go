package queue

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectTraceContext copies envelope metadata before adding W3C trace context.
// This keeps a caller-owned metadata map immutable and lets a transactional
// outbox retain the producer trace across process and retry boundaries.
func InjectTraceContext(ctx context.Context, envelope Envelope) Envelope {
	envelope.Metadata = cloneMetadata(envelope.Metadata)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(envelope.Metadata))
	return envelope
}

// ExtractTraceContext returns a context linked to trace metadata carried by an
// envelope. Invalid or absent carrier values safely leave the parent unchanged.
func ExtractTraceContext(ctx context.Context, envelope Envelope) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(envelope.Metadata))
}

func startQueueSpan(ctx context.Context, envelope Envelope, operation string, kind trace.SpanKind) (context.Context, trace.Span) {
	return otel.Tracer("github.com/complianceforge/platform/queue").Start(
		ctx,
		"queue "+operation,
		trace.WithSpanKind(kind),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.operation.name", operation),
			attribute.String("messaging.message.id", envelope.ID),
			attribute.Int("messaging.message.retry.count", envelope.Attempt-1),
		),
	)
}

func finishQueueSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "queue operation failed")
	}
	span.End()
}
