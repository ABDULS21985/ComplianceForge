package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthCheckDoesNotDialFromReadinessPath(t *testing.T) {
	service, err := NewRabbitMQService("amqp://worker:secret@203.0.113.1:5672/")
	if err != nil {
		t.Fatalf("NewRabbitMQService() error = %v", err)
	}
	started := time.Now()
	err = service.HealthCheck(context.Background())
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("HealthCheck() error = %v, want ErrClosed", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("HealthCheck() took %s and may have attempted a network dial", elapsed)
	}
}

func TestHealthCheckHonorsCancelledContext(t *testing.T) {
	service, err := NewRabbitMQService("amqp://worker:secret@localhost:5672/")
	if err != nil {
		t.Fatalf("NewRabbitMQService() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.HealthCheck(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("HealthCheck() error = %v, want context.Canceled", err)
	}
}
