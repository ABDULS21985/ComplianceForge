// Package observability provides bounded-cardinality metrics, tracing, and
// internal telemetry endpoints for the API and worker processes.
package observability

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var (
	requestDurationBuckets   = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	operationDurationBuckets = []float64{0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 15, 60, 300, 900}
	messageAgeBuckets        = []float64{0.1, 1, 5, 15, 60, 300, 900, 3600, 21600, 86400}
)

// Metrics is intentionally the only application metric registry. Every
// variable label is normalized to a small allowlist or a registered route/task
// name; tenant, user, message, and other unbounded identifiers are never used.
type Metrics struct {
	registry *prometheus.Registry

	httpRequests    *prometheus.CounterVec
	httpDuration    *prometheus.HistogramVec
	httpInFlight    prometheus.Gauge
	dependencyUp    *prometheus.GaugeVec
	dependencyRuns  *prometheus.CounterVec
	dependencyTime  *prometheus.HistogramVec
	queueOperations *prometheus.CounterVec
	queueDuration   *prometheus.HistogramVec
	queueMessageAge prometheus.Histogram
	queueReconnects *prometheus.CounterVec
	workerRuns      *prometheus.CounterVec
	workerDuration  *prometheus.HistogramVec
	workerActive    prometheus.Gauge

	taskMu       sync.RWMutex
	allowedTasks map[string]struct{}
}

func NewMetrics(serviceName, serviceVersion, environment string) *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "complianceforge", Subsystem: "http", Name: "requests_total",
			Help: "HTTP requests completed by method, stable route pattern, and status class.",
		}, []string{"method", "route", "status_class"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "complianceforge", Subsystem: "http", Name: "request_duration_seconds",
			Help: "HTTP request latency by method and stable route pattern.", Buckets: requestDurationBuckets,
		}, []string{"method", "route"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "complianceforge", Subsystem: "http", Name: "requests_in_flight",
			Help: "Number of HTTP requests currently in flight.",
		}),
		dependencyUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "complianceforge", Name: "dependency_up",
			Help: "Whether the most recent bounded dependency health check succeeded.",
		}, []string{"dependency"}),
		dependencyRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "complianceforge", Subsystem: "dependency", Name: "checks_total",
			Help: "Dependency health checks by dependency and outcome.",
		}, []string{"dependency", "outcome"}),
		dependencyTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "complianceforge", Subsystem: "dependency", Name: "check_duration_seconds",
			Help: "Dependency health-check latency.", Buckets: operationDurationBuckets,
		}, []string{"dependency"}),
		queueOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "complianceforge", Subsystem: "queue", Name: "operations_total",
			Help: "Queue operations by bounded operation and outcome.",
		}, []string{"operation", "outcome"}),
		queueDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "complianceforge", Subsystem: "queue", Name: "operation_duration_seconds",
			Help: "Queue operation latency by bounded operation and outcome.", Buckets: operationDurationBuckets,
		}, []string{"operation", "outcome"}),
		queueMessageAge: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "complianceforge", Subsystem: "queue", Name: "message_age_seconds",
			Help: "Age of valid messages when processing begins.", Buckets: messageAgeBuckets,
		}),
		queueReconnects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "complianceforge", Subsystem: "queue", Name: "reconnects_total",
			Help: "Queue reconnect attempts by bounded component.",
		}, []string{"component"}),
		workerRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "complianceforge", Subsystem: "worker", Name: "task_runs_total",
			Help: "Distributed worker task runs by registered task and outcome.",
		}, []string{"task", "outcome"}),
		workerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "complianceforge", Subsystem: "worker", Name: "task_duration_seconds",
			Help: "Distributed worker task runtime by registered task and outcome.", Buckets: operationDurationBuckets,
		}, []string{"task", "outcome"}),
		workerActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "complianceforge", Subsystem: "worker", Name: "active_tasks",
			Help: "Number of acquired worker tasks currently executing.",
		}),
		allowedTasks: make(map[string]struct{}),
	}
	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "complianceforge", Name: "build_info",
			Help: "Static build and runtime identity.",
			ConstLabels: prometheus.Labels{
				"service": normalizeIdentity(serviceName), "version": normalizeIdentity(serviceVersion), "environment": normalizeIdentity(environment),
			},
		}, func() float64 { return 1 }),
		metrics.httpRequests, metrics.httpDuration, metrics.httpInFlight,
		metrics.dependencyUp, metrics.dependencyRuns, metrics.dependencyTime,
		metrics.queueOperations, metrics.queueDuration, metrics.queueMessageAge,
		metrics.queueReconnects, metrics.workerRuns, metrics.workerDuration,
		metrics.workerActive,
	)
	return metrics
}

func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

func (m *Metrics) RegisterPostgresPool(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	collectors := []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "connections_max", Help: "Configured maximum PostgreSQL pool connections."}, func() float64 { return float64(pool.Stat().MaxConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "connections_total", Help: "Total PostgreSQL pool connections."}, func() float64 { return float64(pool.Stat().TotalConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "connections_idle", Help: "Idle PostgreSQL pool connections."}, func() float64 { return float64(pool.Stat().IdleConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "connections_acquired", Help: "Acquired PostgreSQL pool connections."}, func() float64 { return float64(pool.Stat().AcquiredConns()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "acquire_total", Help: "Cumulative PostgreSQL pool acquisitions."}, func() float64 { return float64(pool.Stat().AcquireCount()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "acquire_wait_seconds_total", Help: "Cumulative time waiting for a PostgreSQL pool connection."}, func() float64 { return pool.Stat().AcquireDuration().Seconds() }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "postgres", Name: "empty_acquire_total", Help: "PostgreSQL pool acquisitions that waited for a connection."}, func() float64 { return float64(pool.Stat().EmptyAcquireCount()) }),
	}
	for _, collector := range collectors {
		m.registry.MustRegister(collector)
	}
}

func (m *Metrics) RegisterRedis(client *redis.Client) {
	if client == nil {
		return
	}
	collectors := []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "connections_total", Help: "Total Redis client pool connections."}, func() float64 { return float64(client.PoolStats().TotalConns) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "connections_idle", Help: "Idle Redis client pool connections."}, func() float64 { return float64(client.PoolStats().IdleConns) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "connections_stale", Help: "Stale Redis client pool connections."}, func() float64 { return float64(client.PoolStats().StaleConns) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "pool_hits_total", Help: "Redis pool hits."}, func() float64 { return float64(client.PoolStats().Hits) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "pool_misses_total", Help: "Redis pool misses."}, func() float64 { return float64(client.PoolStats().Misses) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: "complianceforge", Subsystem: "redis", Name: "pool_timeouts_total", Help: "Redis pool timeouts."}, func() float64 { return float64(client.PoolStats().Timeouts) }),
	}
	for _, collector := range collectors {
		m.registry.MustRegister(collector)
	}
}

func (m *Metrics) RegisterWorkerTasks(taskNames []string) {
	m.taskMu.Lock()
	defer m.taskMu.Unlock()
	for _, taskName := range taskNames {
		if validBoundedName(taskName) {
			m.allowedTasks[taskName] = struct{}{}
		}
	}
}

func (m *Metrics) WrapDependencyCheck(name string, check func(context.Context) error) func(context.Context) error {
	dependency := normalizeDependency(name)
	return func(ctx context.Context) error {
		started := time.Now()
		err := check(ctx)
		outcome := "success"
		up := 1.0
		if err != nil {
			outcome = "error"
			up = 0
		}
		m.dependencyUp.WithLabelValues(dependency).Set(up)
		m.dependencyRuns.WithLabelValues(dependency, outcome).Inc()
		m.dependencyTime.WithLabelValues(dependency).Observe(time.Since(started).Seconds())
		return err
	}
}

// The next three methods satisfy queue.Observer without importing the queue
// package and creating a cycle.
func (m *Metrics) ObserveQueueOperation(operation, outcome string, duration time.Duration) {
	operation = normalizeQueueOperation(operation)
	outcome = normalizeOutcome(outcome)
	m.queueOperations.WithLabelValues(operation, outcome).Inc()
	m.queueDuration.WithLabelValues(operation, outcome).Observe(duration.Seconds())
}

func (m *Metrics) ObserveQueueMessageAge(age time.Duration) {
	if age < 0 {
		age = 0
	}
	m.queueMessageAge.Observe(age.Seconds())
}

func (m *Metrics) ObserveQueueReconnect(component string) {
	switch component {
	case "publisher", "consumer":
	default:
		component = "other"
	}
	m.queueReconnects.WithLabelValues(component).Inc()
}

func (m *Metrics) ObserveWorkerTask(taskName string, acquired bool, duration time.Duration, err error) {
	taskName = m.normalizeTask(taskName)
	outcome := "success"
	if !acquired {
		outcome = "skipped"
	} else if err != nil {
		outcome = "error"
	}
	m.workerRuns.WithLabelValues(taskName, outcome).Inc()
	m.workerDuration.WithLabelValues(taskName, outcome).Observe(duration.Seconds())
}

func (m *Metrics) WorkerStarted()  { m.workerActive.Inc() }
func (m *Metrics) WorkerFinished() { m.workerActive.Dec() }

func (m *Metrics) normalizeTask(value string) string {
	m.taskMu.RLock()
	_, allowed := m.allowedTasks[value]
	m.taskMu.RUnlock()
	if !allowed {
		return "other"
	}
	return value
}

func normalizeMethod(method string) string {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return method
	default:
		return "OTHER"
	}
}

func normalizeStatusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return strconv.Itoa(status/100) + "xx"
}

func normalizeDependency(value string) string {
	switch value {
	case "postgres", "redis", "rabbitmq":
		return value
	default:
		return "other"
	}
}

func normalizeQueueOperation(value string) string {
	switch value {
	case "publish", "consume", "retry", "dead_letter", "quarantine", "outbox":
		return value
	default:
		return "other"
	}
}

func normalizeOutcome(value string) string {
	switch value {
	case "success", "error", "retry", "dead_letter", "quarantine", "deduplicated", "in_progress", "skipped":
		return value
	default:
		return "other"
	}
}

func normalizeIdentity(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 || !validBoundedName(value) {
		return "unknown"
	}
	return value
}

func validBoundedName(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._-/", character) {
			continue
		}
		return false
	}
	return true
}
