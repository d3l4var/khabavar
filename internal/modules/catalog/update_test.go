package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

// memUpdateStore records the last UpdateProduct call so the tests can assert what the
// service decided to persist.
type memUpdateStore struct {
	store             *memStore // persists the write, like DBStore does
	called            int
	last              Product
	replaceCategories bool
}

func (m *memUpdateStore) UpdateProduct(_ context.Context, p Product, replaceCategories bool) error {
	m.called++
	m.last = p
	m.replaceCategories = replaceCategories
	if mp, ok := m.store.prods[p.ID]; ok {
		mp.Product = p
		if replaceCategories {
			mp.categoryIDs = append([]string(nil), p.CategoryIDs...)
		}
	}
	return nil
}

// newUpdateService builds a service over the in-memory Store plus the update recorder.
func newUpdateService(t *testing.T) (*Service, *memStore, *memUpdateStore) {
	t.Helper()
	store := newMemStore()
	upd := &memUpdateStore{store: store}
	svc := NewService(store)
	svc.SetUpdateStore(upd)
	return svc, store, upd
}

// TestUpdateProductUnblocksPublish is the regression test for the dead-end found in
// the catalogue E2E: a product created without the publish-gate fields could never be
// published because there was no way to supply them afterwards.
func TestUpdateProductUnblocksPublish(t *testing.T) {
	svc, _, upd := newUpdateService(t)
	ctx := context.Background()
	id, err := svc.CreateProduct(ctx, Product{Title: "تشک بادی", Slug: "air-mattress"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Publishing now must fail: the gate is unsatisfied.
	if err := svc.PublishProduct(ctx, id); !errors.Is(err, ErrPublishGateMissing) {
		t.Fatalf("publish before patch: want ErrPublishGateMissing, got %v", err)
	}

	reviewed := mustTime(t)
	rel, src := "SLEEP_AID", "سازنده"
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{
		SleepRelevance:     &rel,
		SpecSource:         &src,
		SpecLastReviewedAt: &reviewed,
	}); err != nil {
		t.Fatalf("patch gate fields: %v", err)
	}
	if upd.called != 1 {
		t.Fatalf("update store called %d times, want 1", upd.called)
	}
	if upd.last.SleepRelevance != rel || upd.last.SpecSource != src || upd.last.SpecLastReviewedAt == nil {
		t.Fatalf("persisted product lost the gate fields: %+v", upd.last)
	}

	if err := svc.PublishProduct(ctx, id); err != nil {
		t.Fatalf("publish after patch: %v", err)
	}
}

func TestUpdateProductRejectsBlankSlug(t *testing.T) {
	svc, _, _ := newUpdateService(t)
	ctx := context.Background()
	id, _ := svc.CreateProduct(ctx, Product{Title: "x", Slug: "x-1"})
	empty := ""
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{Slug: &empty}); !errors.Is(err, ErrSlugRequired) {
		t.Fatalf("want ErrSlugRequired, got %v", err)
	}
}

// A published product must keep a satisfiable gate: clearing a gate field would either
// violate chk_products_publish_gates or silently drop a live product from the store.
func TestUpdateProductRefusesToBreakPublishedGate(t *testing.T) {
	svc, store, _ := newUpdateService(t)
	ctx := context.Background()
	reviewed := mustTime(t)
	id, err := svc.CreateProduct(ctx, Product{
		Title: "بالش", Slug: "pillow-1", SleepRelevance: "SLEEP_AID",
		SpecSource: "سازنده", SpecLastReviewedAt: &reviewed,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.PublishProduct(ctx, id); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if store.prods[id].Status != ProductPublished {
		t.Fatalf("status=%s want PUBLISHED", store.prods[id].Status)
	}

	empty := ""
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{SleepRelevance: &empty}); !errors.Is(err, ErrPublishGateMissing) {
		t.Fatalf("want ErrPublishGateMissing, got %v", err)
	}
}

func TestUpdateProductUnknownID(t *testing.T) {
	svc, _, _ := newUpdateService(t)
	if _, err := svc.UpdateProduct(context.Background(), "nope", ProductPatch{}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("want ErrProductNotFound, got %v", err)
	}
}

// Category links are only replaced when the patch mentions them.
func TestUpdateProductCategoryReplacementIsExplicit(t *testing.T) {
	svc, _, upd := newUpdateService(t)
	ctx := context.Background()
	id, _ := svc.CreateProduct(ctx, Product{Title: "x", Slug: "x-2"})

	title := "new title"
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{Title: &title}); err != nil {
		t.Fatalf("patch title: %v", err)
	}
	if upd.replaceCategories {
		t.Error("categories must not be replaced when the patch omits category_ids")
	}

	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{CategoryIDs: []string{}}); err != nil {
		t.Fatalf("patch categories: %v", err)
	}
	if !upd.replaceCategories {
		t.Error("an explicit category_ids (even empty) must replace the links")
	}
}

func TestUpdateProductWithoutStoreErrors(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.UpdateProduct(context.Background(), "x", ProductPatch{}); err == nil {
		t.Fatal("expected an error when no update store is wired")
	}
}

// mustTime returns a fixed review timestamp for the publish-gate tests.
func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
}

// TestUpdateProduct_PersistsCategoryReplacement guards a silent write failure: the
// store replaces the category links from the product it receives, so the service must
// apply the patch before calling it. Regression: the links were deleted and the
// response echoed the new set while the table stayed empty.
func TestUpdateProduct_PersistsCategoryReplacement(t *testing.T) {
	svc, store, _ := newUpdateService(t)
	catA, catB := "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	store.prods["p1"] = &memProduct{Product: Product{ID: "p1", Title: "t", Slug: "s", Status: ProductDraft}, categoryIDs: []string{catA}}

	patch := ProductPatch{CategoryIDs: []string{catB}}
	if _, err := svc.UpdateProduct(context.Background(), "p1", patch); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	got := store.prods["p1"].categoryIDs
	if len(got) != 1 || got[0] != catB {
		t.Fatalf("persisted categories = %v, want [%s]", got, catB)
	}
}
