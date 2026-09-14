package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/complianceforge/platform/internal/config"
)

func TestHTTPMiddlewareUsesRoutePatternsAndReturnsTraceID(t *testing.T) {
	previous := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
		otel.SetTextMapPropagator(previousPropagator)
	})

	metrics := NewMetrics("test-api", "test", "test")
	router := chi.NewRouter()
	router.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := metrics.HTTPMiddleware(router)
	request := httptest.NewRequest(http.MethodGet, "/items/private-record-123", nil)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("X-Trace-ID"); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-ID = %q", got)
	}
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	serialized := metricLabels(families)
	if !strings.Contains(serialized, "route=/items/{id}") {
		t.Fatalf("metrics do not contain stable route pattern: %s", serialized)
	}
	if strings.Contains(serialized, "private-record-123") {
		t.Fatalf("metrics leaked a raw path identifier: %s", serialized)
	}
}

func TestWorkerTaskLabelsAreAllowlisted(t *testing.T) {
	metrics := NewMetrics("test-worker", "test", "test")
	metrics.RegisterWorkerTasks([]string{"scheduler.known"})
	metrics.ObserveWorkerTask("attacker-controlled-task", true, time.Millisecond, nil)
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	serialized := metricLabels(families)
	if !strings.Contains(serialized, "task=other") || strings.Contains(serialized, "attacker-controlled-task") {
		t.Fatalf("worker task label was not bounded: %s", serialized)
	}
}

func TestInternalServerSeparatesMetricsAndReadiness(t *testing.T) {
	runtime, err := New(context.Background(), config.ObservabilityConfig{
		MetricsEnabled: true, MetricsAddress: "127.0.0.1:0", TraceSampleRatio: 0.1,
		ServiceVersion: "test", ShutdownTimeoutSeconds: 5,
	}, "test-api", "test", "instance")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := runtime.StartMetricsServer(func(context.Context) error { return errors.New("database unavailable") }); err != nil {
		t.Fatalf("StartMetricsServer() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })

	baseURL := "http://" + runtime.listener.Addr().String()
	response, err := http.Get(baseURL + "/metrics") // #nosec G107 -- URL is an in-process loopback listener created above.
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "complianceforge_build_info") {
		t.Fatalf("metrics response status=%d readErr=%v body=%q", response.StatusCode, readErr, body)
	}
	response, err = http.Get(baseURL + "/health/ready") // #nosec G107 -- URL is an in-process loopback listener created above.
	if err != nil {
		t.Fatalf("GET /health/ready: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d", response.StatusCode)
	}
}

func TestInternalMetricsBearerAuthentication(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	tokenFile := filepath.Join(t.TempDir(), "metrics-token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := New(context.Background(), config.ObservabilityConfig{
		MetricsEnabled: true, MetricsAddress: "127.0.0.1:0", MetricsTokenFile: tokenFile,
		TraceSampleRatio: 0.1, ServiceVersion: "test", ShutdownTimeoutSeconds: 5,
	}, "test-api", "test", "instance")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := runtime.StartMetricsServer(nil); err != nil {
		t.Fatalf("StartMetricsServer() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	endpoint := "http://" + runtime.listener.Addr().String() + "/metrics"

	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	response, err := http.DefaultClient.Do(request) // #nosec G107 -- URL is an in-process loopback listener created above.
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated metrics status = %d", response.StatusCode)
	}
	request, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(request) // #nosec G107 -- URL is an in-process loopback listener created above.
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated metrics status = %d", response.StatusCode)
	}
}

func TestProductionMetricsRequireBearerTokenFile(t *testing.T) {
	_, err := New(context.Background(), config.ObservabilityConfig{
		MetricsEnabled: true, MetricsAddress: "0.0.0.0:9091",
		TraceSampleRatio: 0.1, ServiceVersion: "test", ShutdownTimeoutSeconds: 5,
	}, "test-api", "production", "instance")
	if err == nil || !strings.Contains(err.Error(), "TOKEN_FILE") {
		t.Fatalf("New() error = %v, want metrics token requirement", err)
	}
}

func metricLabels(families []*dto.MetricFamily) string {
	var builder strings.Builder
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				builder.WriteString(label.GetName())
				builder.WriteByte('=')
				builder.WriteString(label.GetValue())
				builder.WriteByte('\n')
			}
		}
	}
	return builder.String()
}
