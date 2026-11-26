// update.go adds product editing (PATCH /admin/products/{id}).
//
// Without an edit path a product created without the publish-gate fields could never
// be published: the gate is checked on publish, and the only way to supply
// sleep_relevance / spec_source / spec_last_reviewed_at afterwards is to edit the
// product. That dead-end was found while exercising the catalogue E2E.
//
// Status is deliberately NOT editable here — publication and pausing have their own
// routes with their own gates (MA-1/MA-6), so a PATCH cannot bypass them.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ProductPatch carries the fields a PATCH may change. A nil pointer means "leave as
// is"; an empty string means "clear it" (which is refused for gate fields on a
// published product).
type ProductPatch struct {
	Title              *string
	Slug               *string
	BrandID            *string
	ShortDescription   *string
	Description        *string
	Manufacturer       *string
	CountryOfOrigin    *string
	SleepRelevance     *string
	SpecSource         *string
	SpecLastReviewedAt *time.Time
	CategoryIDs        []string // nil = leave links untouched; empty slice = clear all
}

// ProductUpdateStore persists an edited product. Separate from Store so the existing
// unit-test fake stays minimal; DBStore implements it.
type ProductUpdateStore interface {
	UpdateProduct(ctx context.Context, p Product, replaceCategories bool) error
}

// SetUpdateStore wires the product editor (DBStore implements ProductUpdateStore).
func (s *Service) SetUpdateStore(us ProductUpdateStore) { s.updates = us }

// UpdateProduct applies a patch to an existing product.
//
// Rules:
//   - the product must exist (ErrProductNotFound);
//   - slug may not be blanked (ErrSlugRequired);
//   - a PUBLISHED product may not lose its publish-gate fields, otherwise the DB CHECK
//     would reject the row and the storefront would lose a live product silently.
func (s *Service) UpdateProduct(ctx context.Context, id string, patch ProductPatch) (*Product, error) {
	if s.updates == nil {
		return nil, errors.New("catalog: update store not configured")
	}
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProductNotFound
	}
	if patch.Title != nil {
		p.Title = *patch.Title
	}
	if patch.Slug != nil {
		if *patch.Slug == "" {
			return nil, ErrSlugRequired
		}
		p.Slug = *patch.Slug
	}
	if patch.BrandID != nil {
		p.BrandID = *patch.BrandID
	}
	if patch.ShortDescription != nil {
		p.ShortDescription = *patch.ShortDescription
	}
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Manufacturer != nil {
		p.Manufacturer = *patch.Manufacturer
	}
	if patch.CountryOfOrigin != nil {
		p.CountryOfOrigin = *patch.CountryOfOrigin
	}
	if patch.SleepRelevance != nil {
		p.SleepRelevance = *patch.SleepRelevance
	}
	if patch.SpecSource != nil {
		p.SpecSource = *patch.SpecSource
	}
	if patch.SpecLastReviewedAt != nil {
		p.SpecLastReviewedAt = patch.SpecLastReviewedAt
	}
	if p.Status == ProductPublished &&
		(p.SleepRelevance == "" || p.SpecSource == "" || p.SpecLastReviewedAt == nil) {
		return nil, ErrPublishGateMissing
	}
	if err := s.updates.UpdateProduct(ctx, *p, patch.CategoryIDs != nil); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}
	if patch.CategoryIDs != nil {
		p.CategoryIDs = patch.CategoryIDs
	}
	return p, nil
}
