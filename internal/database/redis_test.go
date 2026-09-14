package database

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/config"
)

func TestRedisOptionsFromURL(t *testing.T) {
	options, err := RedisOptions(config.RedisConfig{URL: "rediss://app:secret@redis.example.com:6380/4"})
	if err != nil {
		t.Fatalf("RedisOptions() error = %v", err)
	}
	if options.Addr != "redis.example.com:6380" || options.Username != "app" || options.Password != "secret" || options.DB != 4 {
		t.Fatalf("options = %#v", options)
	}
	if options.TLSConfig == nil || options.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS options = %#v", options.TLSConfig)
	}
	assertBoundedRedisOptions(t, options.DialTimeout, options.ReadTimeout, options.WriteTimeout)
}

func TestRedisOptionsFromSplitConfiguration(t *testing.T) {
	options, err := RedisOptions(config.RedisConfig{Host: "localhost", Port: 6379, Password: "secret", DB: 2})
	if err != nil {
		t.Fatalf("RedisOptions() error = %v", err)
	}
	if options.Addr != "localhost:6379" || options.Password != "secret" || options.DB != 2 || options.TLSConfig != nil {
		t.Fatalf("options = %#v", options)
	}
	assertBoundedRedisOptions(t, options.DialTimeout, options.ReadTimeout, options.WriteTimeout)
}

func TestRedisOptionsRejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []config.RedisConfig{
		{URL: "://bad"},
		{Host: "", Port: 6379},
		{Host: "localhost", Port: 0},
	} {
		if _, err := RedisOptions(cfg); err == nil {
			t.Fatalf("RedisOptions(%#v) unexpectedly succeeded", cfg)
		}
	}
}

func assertBoundedRedisOptions(t *testing.T, dial, read, write time.Duration) {
	t.Helper()
	if dial <= 0 || read <= 0 || write <= 0 {
		t.Fatalf("unbounded timeouts: dial=%s read=%s write=%s", dial, read, write)
	}
}
