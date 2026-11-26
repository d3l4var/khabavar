package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func recordingHandler(t *testing.T) (Handler, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	h := func(_ context.Context, e Event) error {
		mu.Lock()
		calls = append(calls, e.ID)
		mu.Unlock()
		return nil
	}
	get := func() []string {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]string, len(calls))
		copy(cp, calls)
		return cp
	}
	return h, get
}

func TestDrain_DispatchesAndMarksPublished(t *testing.T) {
	store := NewMemOutbox()
	idA := store.Seed(Event{EventType: "OrderConfirmed", AggregateID: "ord-1"})
	idB := store.Seed(Event{EventType: "OrderConfirmed", AggregateID: "ord-2"})

	h, getCalls := recordingHandler(t)
	d := NewDispatcher(store, nil, 10, 5)
	d.Register("OrderConfirmed", h)

	n, err := d.Drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 2 {
		t.Fatalf("processed=%d want 2", n)
	}
	calls := getCalls()
	if len(calls) != 2 {
		t.Fatalf("handler called %d times, want 2", len(calls))
	}
	if calls[0] != idA || calls[1] != idB {
		t.Fatalf("dispatch order = %v want [%s,%s]", calls, idA, idB)
	}
	pub := store.PublishedIDs()
	if len(pub) != 2 {
		t.Fatalf("published=%d want 2", len(pub))
	}
	n2, _ := d.Drain(context.Background())
	if n2 != 0 {
		t.Fatalf("second drain processed=%d want 0", n2)
	}
}

func TestDrain_RetriesOnFailureThenSucceeds(t *testing.T) {
	store := NewMemOutbox()
	id := store.Seed(Event{EventType: "PaymentCaptured"})

	var attempts atomic.Int32
	h := func(_ context.Context, _ Event) error {
		if attempts.Add(1) < 3 {
			return errors.New("transient downstream error")
		}
		return nil
	}
	d := NewDispatcher(store, nil, 10, 5)
	d.Register("PaymentCaptured", h)

	d.Drain(context.Background())
	if rc := store.RetryCount(id); rc != 1 {
		t.Fatalf("after 1st drain retry_count=%d want 1", rc)
	}
	if len(store.PublishedIDs()) != 0 {
		t.Fatal("event must not be published while failing")
	}
	d.Drain(context.Background())
	if rc := store.RetryCount(id); rc != 2 {
		t.Fatalf("after 2nd drain retry_count=%d want 2", rc)
	}
	d.Drain(context.Background())
	if len(store.PublishedIDs()) != 1 {
		t.Fatal("event must be published after handler succeeds")
	}
	if store.PublishedIDs()[0] != id {
		t.Fatalf("published id=%s want %s", store.PublishedIDs()[0], id)
	}
}

func TestDrain_DeadLettersAfterMaxRetries(t *testing.T) {
	store := NewMemOutbox()
	id := store.Seed(Event{EventType: "ShipmentDispatched", RetryCount: 5})

	h := func(_ context.Context, _ Event) error { return errors.New("permanent") }
	d := NewDispatcher(store, nil, 10, 5)
	d.Register("ShipmentDispatched", h)

	d.Drain(context.Background())
	if len(store.PublishedIDs()) != 1 {
		t.Fatalf("poison event must be dead-lettered (published), got %d published", len(store.PublishedIDs()))
	}
	if store.PublishedIDs()[0] != id {
		t.Fatalf("dead-lettered id=%s want %s", store.PublishedIDs()[0], id)
	}
}

func TestDrain_NoHandlerMarksFailedThenDeadLetters(t *testing.T) {
	store := NewMemOutbox()
	id := store.Seed(Event{EventType: "UnknownType"})
	d := NewDispatcher(store, nil, 10, 2)

	d.Drain(context.Background())
	if rc := store.RetryCount(id); rc != 1 {
		t.Fatalf("after 1st drain retry_count=%d want 1", rc)
	}
	d.Drain(context.Background())
	if rc := store.RetryCount(id); rc != 2 {
		t.Fatalf("after 2nd drain retry_count=%d want 2", rc)
	}
	d.Drain(context.Background())
	if len(store.PublishedIDs()) != 1 {
		t.Fatalf("unhandled event must dead-letter after max retries, got %d published", len(store.PublishedIDs()))
	}
}

func TestDrain_FallbackHandlesUnregisteredTypes(t *testing.T) {
	store := NewMemOutbox()
	id := store.Seed(Event{EventType: "InventoryAdjusted"})

	h, getCalls := recordingHandler(t)
	d := NewDispatcher(store, nil, 10, 5)
	d.SetFallback(h)

	d.Drain(context.Background())
	if calls := getCalls(); len(calls) != 1 || calls[0] != id {
		t.Fatalf("fallback not invoked: %v", calls)
	}
	if len(store.PublishedIDs()) != 1 {
		t.Fatal("fallback-handled event must be published")
	}
}

func TestRegister_ReplacesHandler(t *testing.T) {
	store := NewMemOutbox()
	store.Seed(Event{EventType: "E", ID: "e1"})

	var first, second atomic.Int32
	d := NewDispatcher(store, nil, 10, 5)
	d.Register("E", func(context.Context, Event) error { first.Add(1); return nil })
	d.Drain(context.Background())
	d.Register("E", func(context.Context, Event) error { second.Add(1); return nil })
	store.Seed(Event{EventType: "E", ID: "e2"})
	d.Drain(context.Background())

	if first.Load() != 1 || second.Load() != 1 {
		t.Fatalf("first=%d second=%d, want 1 and 1", first.Load(), second.Load())
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	store := NewMemOutbox()
	d := NewDispatcher(store, nil, 10, 5)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx, 50*time.Millisecond)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestNewDispatcher_AppliesDefaults(t *testing.T) {
	store := NewMemOutbox()
	d := NewDispatcher(store, nil, 0, -1)
	if d.batchSize != 100 {
		t.Fatalf("batchSize=%d want 100", d.batchSize)
	}
	if d.maxRetries != 5 {
		t.Fatalf("maxRetries=%d want 5", d.maxRetries)
	}
}
