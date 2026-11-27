package catalog

import (
	"context"
	"errors"
	"testing"
)

// TestPG_UpdateProductPartialPatch covers DBStore.UpdateProduct: a partial patch touches
// only the fields it names, replaceCategories is honoured, and an unknown id is a sentinel
// error. UpdateProduct was at 0%.
func TestPG_UpdateProductPartialPatch(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store)
	svc.SetUpdateStore(store)

	catA, err := svc.CreateCategory(ctx, Category{Name: "الف", Slug: "cat-a"})
	if err != nil {
		t.Fatalf("category A: %v", err)
	}
	catB, err := svc.CreateCategory(ctx, Category{Name: "ب", Slug: "cat-b"})
	if err != nil {
		t.Fatalf("category B: %v", err)
	}
	id, err := svc.CreateProduct(ctx, Product{
		Title: "بالش", Slug: "pillow-patch", ShortDescription: "توضیح کوتاه",
		CategoryIDs: []string{catA},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A patch naming only the title must leave everything else alone.
	title := "بالش تازه"
	updated, err := svc.UpdateProduct(ctx, id, ProductPatch{Title: &title})
	if err != nil {
		t.Fatalf("patch title: %v", err)
	}
	if updated.Title != title || updated.Slug != "pillow-patch" || updated.ShortDescription != "توضیح کوتاه" {
		t.Fatalf("partial patch clobbered a field: %+v", updated)
	}

	// Replacing the category set with an explicit list.
	updated, err = svc.UpdateProduct(ctx, id, ProductPatch{CategoryIDs: []string{catB}})
	if err != nil {
		t.Fatalf("patch categories: %v", err)
	}
	if len(updated.CategoryIDs) != 1 || updated.CategoryIDs[0] != catB {
		t.Fatalf("categories after replace = %v", updated.CategoryIDs)
	}

	// An empty patch is a no-op, not an error.
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{}); err != nil {
		t.Fatalf("empty patch: %v", err)
	}

	// An unknown id is the sentinel, not a driver error.
	if _, err := svc.UpdateProduct(ctx, "00000000-0000-0000-0000-000000000000", ProductPatch{Title: &title}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("unknown id = %v, want ErrProductNotFound", err)
	}

	// A slug collision is a conflict the handler can map, not a raw 500.
	other, err := svc.CreateProduct(ctx, Product{Title: "دیگری", Slug: "pillow-other"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	_ = other
	dup := "pillow-other"
	if _, err := svc.UpdateProduct(ctx, id, ProductPatch{Slug: &dup}); err == nil {
		t.Fatal("duplicate slug was accepted")
	}
}
