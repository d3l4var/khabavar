package shared

import (
	"context"
	"testing"
)

// TestActorIDContract pins the semantics every sensitive handler depends on (IAM §۷): the
// actor survives across module boundaries, an anonymous call yields "", and a caller cannot
// smuggle a non-string value into the slot to make ActorID panic.
func TestActorIDContract(t *testing.T) {
	// Anonymous: the storefront and background jobs must get "" so handlers can refuse.
	if got := ActorID(context.Background()); got != "" {
		t.Fatalf("anonymous actor = %q, want empty", got)
	}
	// A nil context must not panic either.
	//nolint:staticcheck // deliberately exercising the defensive path
	if got := ActorID(nil); got != "" {
		t.Fatalf("nil context actor = %q, want empty", got)
	}

	// Round trip.
	ctx := WithActorID(context.Background(), "admin-1")
	if got := ActorID(ctx); got != "admin-1" {
		t.Fatalf("actor = %q, want admin-1", got)
	}

	// A derived context inherits the actor; an override wins in the child only.
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	if got := ActorID(child); got != "admin-1" {
		t.Fatalf("inherited actor = %q", got)
	}
	grandchild := WithActorID(child, "admin-2")
	if got := ActorID(grandchild); got != "admin-2" {
		t.Fatalf("overridden actor = %q", got)
	}
	if got := ActorID(ctx); got != "admin-1" {
		t.Fatalf("parent actor changed by a child override: %q", got)
	}

	// Two sibling contexts do not leak into each other.
	a := WithActorID(context.Background(), "a")
	b := WithActorID(context.Background(), "b")
	if ActorID(a) != "a" || ActorID(b) != "b" {
		t.Fatalf("sibling leak: a=%q b=%q", ActorID(a), ActorID(b))
	}

	// A wrong type in the slot is "" rather than a panic: the type assertion is comma-ok.
	bad := context.WithValue(context.Background(), actorKey{}, 42)
	if got := ActorID(bad); got != "" {
		t.Fatalf("non-string actor = %q, want empty", got)
	}

	// An empty id is stored as empty, so handlers treat it as anonymous.
	if got := ActorID(WithActorID(context.Background(), "")); got != "" {
		t.Fatalf("empty id = %q", got)
	}
}
