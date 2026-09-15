package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/config"
)

func TestPublicHTTPServerUsesConfiguredBoundedDeadlines(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newPublicHTTPServer(":8181", handler, config.HTTPConfig{
		ReadHeaderTimeoutSeconds: 7,
		ReadTimeoutSeconds:       91,
		WriteTimeoutSeconds:      181,
		IdleTimeoutSeconds:       67,
	})
	if server.Addr != ":8181" || server.Handler == nil || server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("server envelope = %#v", server)
	}
	if server.ReadHeaderTimeout != 7*time.Second || server.ReadTimeout != 91*time.Second ||
		server.WriteTimeout != 181*time.Second || server.IdleTimeout != 67*time.Second {
		t.Fatalf("server deadlines = header=%s read=%s write=%s idle=%s", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}

func TestProductionAPIDatabasePostureIsMandatoryAndFailClosed(t *testing.T) {
	want := errors.New("runtime role is superuser")
	called := false
	err := enforceProductionAPIDatabasePosture(context.Background(), " production ", func(ctx context.Context) error {
		called = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
			t.Fatalf("posture context deadline = %v, ok=%v", deadline, ok)
		}
		return want
	})
	if !called || !errors.Is(err, want) || !strings.Contains(err.Error(), "production API database identity") {
		t.Fatalf("called=%v error=%v", called, err)
	}
}

func TestNonProductionAPIDatabasePostureDoesNotRequireMigration56(t *testing.T) {
	called := false
	err := enforceProductionAPIDatabasePosture(context.Background(), "development", func(context.Context) error {
		called = true
		return errors.New("must not run")
	})
	if err != nil || called {
		t.Fatalf("error=%v called=%v", err, called)
	}
}

func TestProductionAPIDatabasePostureRejectsMissingCheck(t *testing.T) {
	err := enforceProductionAPIDatabasePosture(context.Background(), "production", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error=%v", err)
	}
}
