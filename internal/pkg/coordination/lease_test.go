package coordination

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type leaseStoreRecorder struct {
	mu         sync.Mutex
	active     bool
	renewals   int
	releases   int
	renewErr   error
	releaseErr error
	lastRunErr error
}

func (s *leaseStoreRecorder) TryAcquire(_ context.Context, task string, duration time.Duration) (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return Lease{}, false, nil
	}
	s.active = true
	return Lease{TaskName: task, OwnerID: uuid.NewString(), Token: uuid.NewString(), ExpiresAt: time.Now().Add(duration)}, true, nil
}

func (s *leaseStoreRecorder) Renew(_ context.Context, lease Lease, duration time.Duration) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewals++
	if s.renewErr != nil {
		return Lease{}, s.renewErr
	}
	lease.ExpiresAt = time.Now().Add(duration)
	return lease, nil
}

func (s *leaseStoreRecorder) Release(_ context.Context, _ Lease, runErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases++
	s.lastRunErr = runErr
	if s.releaseErr == nil {
		s.active = false
	}
	return s.releaseErr
}

func (s *leaseStoreRecorder) snapshot() (renewals, releases int, lastErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renewals, s.releases, s.lastRunErr
}

func TestCoordinatorSkipsWorkHeldByAnotherReplica(t *testing.T) {
	store := &leaseStoreRecorder{active: true}
	coordinator, err := NewCoordinator(store, CoordinatorConfig{Lease: time.Second, ReleaseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	acquired, err := coordinator.Run(context.Background(), "reports.generate", func(context.Context) error {
		called = true
		return nil
	})
	if err != nil || acquired || called {
		t.Fatalf("Run() acquired=%t called=%t err=%v", acquired, called, err)
	}
}

func TestCoordinatorRenewsAndReleasesLongRunningWork(t *testing.T) {
	store := &leaseStoreRecorder{}
	coordinator, err := NewCoordinator(store, CoordinatorConfig{Lease: 30 * time.Millisecond, ReleaseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Run(context.Background(), "reports.generate", func(context.Context) error {
		time.Sleep(75 * time.Millisecond)
		return nil
	})
	if err != nil || !acquired {
		t.Fatalf("Run() acquired=%t err=%v", acquired, err)
	}
	renewals, releases, lastErr := store.snapshot()
	if renewals < 2 || releases != 1 || lastErr != nil {
		t.Fatalf("renewals=%d releases=%d lastErr=%v", renewals, releases, lastErr)
	}
}

func TestCoordinatorCancelsWorkWhenLeaseIsLost(t *testing.T) {
	store := &leaseStoreRecorder{renewErr: ErrLeaseLost}
	coordinator, err := NewCoordinator(store, CoordinatorConfig{Lease: 30 * time.Millisecond, ReleaseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Run(context.Background(), "reports.generate", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !acquired || !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Run() acquired=%t err=%v", acquired, err)
	}
	_, releases, lastErr := store.snapshot()
	if releases != 1 || lastErr == nil {
		t.Fatalf("releases=%d lastErr=%v", releases, lastErr)
	}
}

func TestCoordinatorConvertsPanicToErrorAndReleases(t *testing.T) {
	store := &leaseStoreRecorder{}
	coordinator, err := NewCoordinator(store, CoordinatorConfig{Lease: time.Second, ReleaseTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := coordinator.Run(context.Background(), "reports.generate", func(context.Context) error {
		panic("broken report")
	})
	if !acquired || err == nil {
		t.Fatalf("Run() acquired=%t err=%v", acquired, err)
	}
	_, releases, lastErr := store.snapshot()
	if releases != 1 || lastErr == nil {
		t.Fatalf("releases=%d lastErr=%v", releases, lastErr)
	}
}
