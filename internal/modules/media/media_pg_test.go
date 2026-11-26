package media

import (
	"context"
	"testing"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// TestPG_AssetRoundTrip: media had no PG test at all, so the asset table and its
// columns were only ever exercised through the in-memory store.
func TestPG_AssetRoundTrip(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewDBStore(pool)

	id, err := store.CreateAsset(ctx, Asset{
		MediaType:  "IMAGE",
		StorageKey: "products/pillow-1.webp",
		AltText:    "بالش طبی",
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	got, err := store.GetAsset(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got.StorageKey != "products/pillow-1.webp" || got.MediaType != "IMAGE" {
		t.Fatalf("asset = %+v, want the stored key and type", got)
	}
	if got.AltText != "بالش طبی" {
		t.Fatalf("alt_text = %q, want the Persian alt text", got.AltText)
	}
}
