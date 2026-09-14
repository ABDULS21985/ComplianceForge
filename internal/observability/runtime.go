package observability

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/complianceforge/platform/internal/config"
)

type Runtime struct {
	config         config.ObservabilityConfig
	metrics        *Metrics
	tracerProvider *sdktrace.TracerProvider
	metricsToken   []byte

	serverMu sync.Mutex
	server   *http.Server
	listener net.Listener
	errors   chan error
	draining atomic.Bool
	shutdown sync.Once
}

func New(ctx context.Context, cfg config.ObservabilityConfig, serviceName, environment, instanceID string) (*Runtime, error) {
	metrics := NewMetrics(serviceName, cfg.ServiceVersion, environment)
	runtime := &Runtime{config: cfg, metrics: metrics, errors: make(chan error, 1)}
	if cfg.MetricsEnabled && (environment == "staging" || environment == "production") && cfg.MetricsTokenFile == "" {
		return nil, errors.New("OBSERVABILITY_METRICS_TOKEN_FILE is required for a production metrics listener")
	}
	if cfg.MetricsEnabled && cfg.MetricsTokenFile != "" {
		token, err := readMetricsToken(cfg.MetricsTokenFile)
		if err != nil {
			return nil, err
		}
		runtime.metricsToken = token
	}

	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})
	otel.SetTextMapPropagator(propagator)
	if !cfg.TracingEnabled {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return runtime, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPTraceEndpoint),
		otlptracehttp.WithTimeout(5*time.Second),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled: true, InitialInterval: time.Second, MaxInterval: 5 * time.Second, MaxElapsedTime: 30 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", cfg.ServiceVersion),
		attribute.String("deployment.environment.name", environment),
		attribute.String("service.instance.id", instanceID),
	))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(time.Second),
			sdktrace.WithExportTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))),
		sdktrace.WithResource(res),
	)
	runtime.tracerProvider = provider
	otel.SetTracerProvider(provider)
	return runtime, nil
}

func (r *Runtime) Metrics() *Metrics { return r.metrics }

// StartMetricsServer binds synchronously, so invalid/busy addresses fail
// startup rather than producing an apparently healthy process with no metrics.
// The listener contains only telemetry health and metrics endpoints and must not
// be routed through the public API ingress.
func (r *Runtime) StartMetricsServer(readiness func(context.Context) error) error {
	if !r.config.MetricsEnabled {
		return nil
	}
	r.serverMu.Lock()
	defer r.serverMu.Unlock()
	if r.server != nil {
		return errors.New("observability metrics server is already running")
	}
	listener, err := net.Listen("tcp", r.config.MetricsAddress)
	if err != nil {
		return fmt.Errorf("listen on internal observability address %s: %w", r.config.MetricsAddress, err)
	}
	mux := http.NewServeMux()
	metricsHandler := promhttp.HandlerFor(r.metrics.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		Timeout:           10 * time.Second,
	})
	mux.Handle("GET /metrics", r.authorizeMetrics(metricsHandler))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, http.StatusOK, "live")
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, request *http.Request) {
		if r.draining.Load() {
			writeHealth(w, http.StatusServiceUnavailable, "draining")
			return
		}
		if readiness != nil {
			ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
			err := readiness(ctx)
			cancel()
			if err != nil {
				writeHealth(w, http.StatusServiceUnavailable, "unready")
				return
			}
		}
		writeHealth(w, http.StatusOK, "ready")
	})
	r.listener = listener
	r.server = &http.Server{
		Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() {
		if serveErr := r.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case r.errors <- fmt.Errorf("internal observability server: %w", serveErr):
			default:
			}
		}
	}()
	return nil
}

func (r *Runtime) authorizeMetrics(next http.Handler) http.Handler {
	if len(r.metricsToken) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		scheme, value, found := strings.Cut(request.Header.Get("Authorization"), " ")
		provided := []byte(value)
		if !found || !strings.EqualFold(scheme, "Bearer") || len(provided) != len(r.metricsToken) || subtle.ConstantTimeCompare(provided, r.metricsToken) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func readMetricsToken(path string) ([]byte, error) {
	directory, name := filepath.Split(path)
	if directory == "" || name == "" {
		return nil, fmt.Errorf("metrics token file must be an absolute file path")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open metrics token directory: %w", err)
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open metrics token file: %w", err)
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return nil, fmt.Errorf("read metrics token file: %w", err)
	}
	token := []byte(strings.TrimSpace(string(payload)))
	if len(token) < 32 || len(token) > 4096 {
		return nil, fmt.Errorf("metrics bearer token must contain between 32 and 4096 non-whitespace bytes")
	}
	return token, nil
}

func (r *Runtime) Errors() <-chan error { return r.errors }

func (r *Runtime) SetDraining() { r.draining.Store(true) }

func (r *Runtime) Shutdown(ctx context.Context) error {
	var shutdownErr error
	r.shutdown.Do(func() {
		r.SetDraining()
		r.serverMu.Lock()
		server := r.server
		r.serverMu.Unlock()
		if server != nil {
			shutdownErr = errors.Join(shutdownErr, server.Shutdown(ctx))
		}
		if r.tracerProvider != nil {
			shutdownErr = errors.Join(shutdownErr, r.tracerProvider.Shutdown(ctx))
		}
	})
	return shutdownErr
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}
