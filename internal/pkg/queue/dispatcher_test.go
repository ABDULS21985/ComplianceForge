package queue

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDispatcherRegistrationAndDispatch(t *testing.T) {
	dispatcher := NewDispatcher()
	called := false
	if err := dispatcher.Register("job.run", func(_ context.Context, envelope Envelope) error {
		called = envelope.Type == "job.run"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Register("job.other", func(context.Context, Envelope) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Handle(context.Background(), Envelope{Type: "job.run"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("registered handler was not called")
	}
	if got, want := dispatcher.Types(), []string{"job.other", "job.run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Types() = %v, want %v", got, want)
	}
}

func TestDispatcherRejectsDuplicateAndUnknownTypes(t *testing.T) {
	dispatcher := NewDispatcher()
	handler := func(context.Context, Envelope) error { return nil }
	if err := dispatcher.Register("job.run", handler); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Register("job.run", handler); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	err := dispatcher.Handle(context.Background(), Envelope{Type: "job.missing"})
	var permanent *PermanentError
	if !errors.As(err, &permanent) {
		t.Fatalf("unknown type error = %v, want PermanentError", err)
	}
}
