package queue

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	config := DefaultConfig("amqps://worker:secret@rabbitmq.example.com:5671/grc")
	if err := config.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() error = %v", err)
	}
	if config.QueueType != "quorum" || config.MaxAttempts != 5 || config.Prefetch != 16 {
		t.Fatalf("unexpected durability defaults: %+v", config)
	}
}

func TestConfigFromEnvironment(t *testing.T) {
	t.Setenv("QUEUE_PREFETCH", "7")
	t.Setenv("QUEUE_MAX_ATTEMPTS", "3")
	t.Setenv("QUEUE_RETRY_DELAY", "45s")
	t.Setenv("QUEUE_MAX_MESSAGE_BYTES", "2048")
	config, err := ConfigFromEnvironment("amqp://worker:secret@localhost:5672/grc")
	if err != nil {
		t.Fatalf("ConfigFromEnvironment() error = %v", err)
	}
	if config.Prefetch != 7 || config.MaxAttempts != 3 || config.RetryDelay != 45*time.Second || config.MaxMessageBytes != 2048 {
		t.Fatalf("environment overrides not applied: %+v", config)
	}
}

func TestConfigFromEnvironmentRejectsMalformedValues(t *testing.T) {
	t.Setenv("QUEUE_PREFETCH", "many")
	if _, err := ConfigFromEnvironment("amqp://localhost:5672/"); err == nil {
		t.Fatal("expected malformed prefetch error")
	}
}

func TestRedactedEndpointOmitsCredentials(t *testing.T) {
	endpoint := redactedEndpoint("amqps://worker:super-secret@rabbitmq.example.com:5671/grc")
	if strings.Contains(endpoint, "worker") || strings.Contains(endpoint, "super-secret") {
		t.Fatalf("redacted endpoint leaked credentials: %s", endpoint)
	}
	if endpoint != "amqps://rabbitmq.example.com:5671/grc" {
		t.Fatalf("redacted endpoint = %q", endpoint)
	}
}

func TestConfigValidationRejectsUnsafeOptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "URL", mutate: func(c *Config) { c.URL = "http://rabbitmq" }},
		{name: "queue type", mutate: func(c *Config) { c.QueueType = "transient" }},
		{name: "prefetch", mutate: func(c *Config) { c.Prefetch = 0 }},
		{name: "prefetch overflow", mutate: func(c *Config) { c.Prefetch = 65_536 }},
		{name: "attempts", mutate: func(c *Config) { c.MaxAttempts = 0 }},
		{name: "retry", mutate: func(c *Config) { c.RetryDelay = 0 }},
		{name: "reconnect", mutate: func(c *Config) { c.ReconnectMax = c.ReconnectMin / 2 }},
		{name: "exchange", mutate: func(c *Config) { c.Exchange = "bad exchange" }},
		{name: "duplicate exchanges", mutate: func(c *Config) { c.RetryExchange = c.Exchange }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig("amqp://localhost:5672/")
			tt.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
