package queue

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultExchange           = "complianceforge.jobs"
	DefaultRetryExchange      = "complianceforge.jobs.retry"
	DefaultDeadLetterExchange = "complianceforge.jobs.dead"
	DefaultQuarantineExchange = "complianceforge.jobs.quarantine"
)

var brokerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// Config controls RabbitMQ durability, delivery, and reconnect behavior.
type Config struct {
	URL                    string
	Exchange               string
	RetryExchange          string
	DeadLetterExchange     string
	QuarantineExchange     string
	QueueType              string
	ConnectionName         string
	Prefetch               int
	MaxAttempts            int
	RetryDelay             time.Duration
	PublishTimeout         time.Duration
	ConnectTimeout         time.Duration
	Heartbeat              time.Duration
	ReconnectMin           time.Duration
	ReconnectMax           time.Duration
	MaxMessageBytes        int
	IdempotencyLease       time.Duration
	IdempotencyTTL         time.Duration
	IdempotencyMaxMessages int
}

// DefaultConfig returns production-safe defaults. Quorum queues survive node
// loss, publisher confirms prevent silent loss, and retries are bounded.
func DefaultConfig(amqpURL string) Config {
	return Config{
		URL:                    amqpURL,
		Exchange:               DefaultExchange,
		RetryExchange:          DefaultRetryExchange,
		DeadLetterExchange:     DefaultDeadLetterExchange,
		QuarantineExchange:     DefaultQuarantineExchange,
		QueueType:              "quorum",
		ConnectionName:         "complianceforge-worker",
		Prefetch:               16,
		MaxAttempts:            5,
		RetryDelay:             30 * time.Second,
		PublishTimeout:         10 * time.Second,
		ConnectTimeout:         10 * time.Second,
		Heartbeat:              15 * time.Second,
		ReconnectMin:           time.Second,
		ReconnectMax:           30 * time.Second,
		MaxMessageBytes:        1024 * 1024,
		IdempotencyLease:       5 * time.Minute,
		IdempotencyTTL:         24 * time.Hour,
		IdempotencyMaxMessages: 100_000,
	}
}

// ConfigFromEnvironment overlays optional worker-specific queue tuning without
// expanding the application's core configuration contract.
func ConfigFromEnvironment(amqpURL string) (Config, error) {
	config := DefaultConfig(amqpURL)
	stringValues := map[string]*string{
		"QUEUE_EXCHANGE":             &config.Exchange,
		"QUEUE_RETRY_EXCHANGE":       &config.RetryExchange,
		"QUEUE_DEAD_LETTER_EXCHANGE": &config.DeadLetterExchange,
		"QUEUE_QUARANTINE_EXCHANGE":  &config.QuarantineExchange,
		"QUEUE_TYPE":                 &config.QueueType,
		"QUEUE_CONNECTION_NAME":      &config.ConnectionName,
	}
	for name, destination := range stringValues {
		if value, ok := os.LookupEnv(name); ok {
			*destination = strings.TrimSpace(value)
		}
	}

	integerValues := map[string]*int{
		"QUEUE_PREFETCH":                 &config.Prefetch,
		"QUEUE_MAX_ATTEMPTS":             &config.MaxAttempts,
		"QUEUE_MAX_MESSAGE_BYTES":        &config.MaxMessageBytes,
		"QUEUE_IDEMPOTENCY_MAX_MESSAGES": &config.IdempotencyMaxMessages,
	}
	for name, destination := range integerValues {
		if value, ok := os.LookupEnv(name); ok {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return Config{}, fmt.Errorf("%s must be an integer: %w", name, err)
			}
			*destination = parsed
		}
	}

	durationValues := map[string]*time.Duration{
		"QUEUE_RETRY_DELAY":       &config.RetryDelay,
		"QUEUE_PUBLISH_TIMEOUT":   &config.PublishTimeout,
		"QUEUE_CONNECT_TIMEOUT":   &config.ConnectTimeout,
		"QUEUE_HEARTBEAT":         &config.Heartbeat,
		"QUEUE_RECONNECT_MIN":     &config.ReconnectMin,
		"QUEUE_RECONNECT_MAX":     &config.ReconnectMax,
		"QUEUE_IDEMPOTENCY_LEASE": &config.IdempotencyLease,
		"QUEUE_IDEMPOTENCY_TTL":   &config.IdempotencyTTL,
	}
	for name, destination := range durationValues {
		if value, ok := os.LookupEnv(name); ok {
			parsed, err := time.ParseDuration(strings.TrimSpace(value))
			if err != nil {
				return Config{}, fmt.Errorf("%s must be a Go duration: %w", name, err)
			}
			*destination = parsed
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	parsed, err := url.Parse(c.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") {
		return fmt.Errorf("RabbitMQ URL must be a valid amqp or amqps URL")
	}
	for label, name := range map[string]string{
		"exchange": c.Exchange, "retry exchange": c.RetryExchange,
		"dead-letter exchange": c.DeadLetterExchange,
		"quarantine exchange":  c.QuarantineExchange,
	} {
		if err := validateBrokerName(name, 255); err != nil {
			return fmt.Errorf("invalid %s: %w", label, err)
		}
	}
	exchanges := []string{c.Exchange, c.RetryExchange, c.DeadLetterExchange, c.QuarantineExchange}
	seenExchanges := make(map[string]struct{}, len(exchanges))
	for _, exchange := range exchanges {
		if _, exists := seenExchanges[exchange]; exists {
			return fmt.Errorf("main, retry, dead-letter, and quarantine exchanges must be distinct")
		}
		seenExchanges[exchange] = struct{}{}
	}
	if c.QueueType != "quorum" && c.QueueType != "classic" {
		return fmt.Errorf("queue type must be quorum or classic")
	}
	if strings.TrimSpace(c.ConnectionName) == "" || len(c.ConnectionName) > 255 {
		return fmt.Errorf("connection name is required and must not exceed 255 bytes")
	}
	if c.Prefetch < 1 || c.Prefetch > 65_535 {
		return fmt.Errorf("prefetch must be between 1 and 65535")
	}
	if c.MaxAttempts < 1 || int64(c.MaxAttempts) > maxAMQPSignedInteger-2 {
		return fmt.Errorf("max attempts must be between 1 and %d", maxAMQPSignedInteger-2)
	}
	if c.RetryDelay < time.Second {
		return fmt.Errorf("retry delay must be at least one second")
	}
	if c.PublishTimeout <= 0 || c.ConnectTimeout <= 0 || c.Heartbeat <= 0 {
		return fmt.Errorf("publish timeout, connect timeout, and heartbeat must be greater than zero")
	}
	if c.ReconnectMin <= 0 || c.ReconnectMax < c.ReconnectMin {
		return fmt.Errorf("reconnect bounds are invalid")
	}
	if c.MaxMessageBytes < 1 {
		return fmt.Errorf("max message bytes must be greater than zero")
	}
	if c.IdempotencyLease < 3*time.Second || c.IdempotencyTTL < c.IdempotencyLease || c.IdempotencyMaxMessages < 1 {
		return fmt.Errorf("idempotency lease must be at least three seconds, TTL must be at least the lease, and capacity must be greater than zero")
	}
	return nil
}

func validateBrokerName(name string, maxLength int) error {
	if len(name) == 0 || len(name) > maxLength || !brokerNamePattern.MatchString(name) {
		return fmt.Errorf("name must match %s and contain at most %d bytes", brokerNamePattern, maxLength)
	}
	return nil
}

func redactedEndpoint(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "<invalid RabbitMQ endpoint>"
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()
}
