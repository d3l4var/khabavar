// Package catalog implements the catalog module (SAD §۴.۲, DB v2 §۶, R-04): brands,
// categories (tree), products, sellable variants, and the structured attribute
// dictionary. Entities: brands, categories, products, product_categories,
// product_variants, attribute_definitions, attribute_options,
// product_attribute_values, product_media, media_assets.
//
// The signature business rule is the publish gate (PRD §۲ MA-1 / MA-6): a product may
// move to PUBLISHED only when it carries a non-empty sleep_relevance (real sleep
// justification), a non-empty spec_source (technical-spec provenance), and a
// spec_last_reviewed_at timestamp. The DB enforces this with a CHECK constraint; the
// Service enforces it first and returns a clear domain error so callers do not depend
// on a constraint violation. Variant SKUs are unique (§۱۵) and checked at creation.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Status enums (mirrors the CHECK constraints in 000002_catalog.up.sql).
const (
	CategoryStatusActive   = "ACTIVE"
	CategoryStatusInactive = "INACTIVE"

	ProductDraft         = "DRAFT"
	ProductDataReview    = "DATA_REVIEW"
	ProductApproved      = "APPROVED"
	ProductPublished     = "PUBLISHED"
	ProductPaused        = "PAUSED"
	ProductDiscontinued  = "DISCONTINUED"

	VariantDraft        = "DRAFT"
	VariantActive       = "ACTIVE"
	VariantInactive     = "INACTIVE"
	VariantDiscontinued = "DISCONTINUED"
)

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrCategoryNotFound   = errors.New("catalog: category not found")
	ErrProductNotFound    = errors.New("catalog: product not found")
	ErrVariantNotFound    = errors.New("catalog: variant not found")
	ErrPublishGateMissing = errors.New("catalog: publish requires sleep_relevance, spec_source and spec_last_reviewed_at (MA-1/MA-6)")
	ErrInvalidStatus      = errors.New("catalog: invalid status transition")
	ErrDuplicateSKU       = errors.New("catalog: variant sku already exists")
	ErrSlugRequired       = errors.New("catalog: slug required")
)

// Category mirrors categories (parent_id forms the tree).
type Category struct {
	ID          string
	ParentID    string
	Name        string
	Slug        string
	Description string
	ImageID     string
	SortOrder   int
	Status      string
}

// CategoryNode is a category with its children (built by Service.CategoryTree).
type CategoryNode struct {
	Category
	Children []CategoryNode `json:"children,omitempty"`
}

// Product mirrors products. SleepRelevance/SpecSource/SpecLastReviewedAt are the
// publish-gate fields (PRD §۲ MA-1/MA-6).
type Product struct {
	ID                 string
	Title              string
	Slug               string
	BrandID            string
	ShortDescription   string
	Description        string
	Manufacturer       string
	CountryOfOrigin    string
	SleepRelevance     string
	SpecSource         string
	SpecLastReviewedAt *time.Time
	Status             string
	PublishedAt        *time.Time
	CategoryIDs        []string
}

// Variant mirrors product_variants — the sellable unit (R-04 §۵). SKU is unique (§۱۵).
type Variant struct {
	ID          string
	ProductID   string
	SKU         string
	Barcode     string
	Attributes  json.RawMessage
	WeightGrams *int
	LengthMm    *int
	WidthMm     *int
	HeightMm    *int
	Status      string
}

// ProductDetail is a product with its variants (the product-page payload).
type ProductDetail struct {
	Product
	Variants []Variant `json:"variants"`
}

// Store is the persistence boundary for catalog. The DB implementation uses pgx; unit
// tests use an in-memory implementation (catalog_test.go).
type Store interface {
	CreateCategory(ctx context.Context, c Category) (string, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateProduct(ctx context.Context, p Product) (string, error)
	GetProduct(ctx context.Context, id string) (*Product, error)
	// ListProductsByCategory returns products attached to categoryID, filtered by
	// status (empty string = any). page is 1-based; pageSize is clamped by the caller.
	ListProductsByCategory(ctx context.Context, categoryID, status string, page, pageSize int) ([]Product, int, error)
	UpdateProductStatus(ctx context.Context, id, status string, publishedAt *time.Time) error
	CreateVariant(ctx context.Context, v Variant) (string, error)
	ListVariantsByProduct(ctx context.Context, productID string) ([]Variant, error)
	GetVariantBySKU(ctx context.Context, sku string) (*Variant, error)
}

// Service is the catalog module boundary (SAD §۴.۲).
type Service struct {
	store  Store
	search SearchStore // optional; nil → fallback linear scan (tests only)
	now    func() time.Time
}

// SetSearchStore wires the search backend (DBStore implements SearchStore).
// If not called, SearchProducts falls back to a linear scan (tests only).
func (s *Service) SetSearchStore(ss SearchStore) {
	s.search = ss
}

// NewService wires the store. now is injected for deterministic tests.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// CreateCategory persists a category. ParentID may be empty for a root.
func (s *Service) CreateCategory(ctx context.Context, c Category) (string, error) {
	if c.Slug == "" {
		return "", ErrSlugRequired
	}
	if c.Status == "" {
		c.Status = CategoryStatusActive
	}
	return s.store.CreateCategory(ctx, c)
}

// CategoryTree loads all categories and assembles them into a forest ordered by
// SortOrder then Name. Roots are categories with an empty ParentID.
func (s *Service) CategoryTree(ctx context.Context) ([]CategoryNode, error) {
	flat, err := s.store.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return buildCategoryTree(flat), nil
}

// buildCategoryTree assembles a forest from a flat category slice. A category is a
// root when its ParentID is empty or refers to a category not present in the slice
// (orphan-safe). Children are built depth-first so nested descendants are preserved.
func buildCategoryTree(flat []Category) []CategoryNode {
	present := make(map[string]bool, len(flat))
	for _, c := range flat {
		present[c.ID] = true
	}
	childrenOf := make(map[string][]Category)
	var roots []Category
	seenRoot := make(map[string]bool)
	for _, c := range flat {
		if c.ParentID == "" || !present[c.ParentID] {
			if !seenRoot[c.ID] {
				seenRoot[c.ID] = true
				roots = append(roots, c)
			}
			continue
		}
		childrenOf[c.ParentID] = append(childrenOf[c.ParentID], c)
	}
	var build func(c Category) CategoryNode
	build = func(c Category) CategoryNode {
		node := CategoryNode{Category: c}
		for _, ch := range childrenOf[c.ID] {
			node.Children = append(node.Children, build(ch))
		}
		return node
	}
	out := make([]CategoryNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, build(r))
	}
	return out
}

// CreateProduct persists a product in DRAFT status (the only valid initial status).
// Slug is required (§۱۵). Publish-gate fields may be empty at creation.
func (s *Service) CreateProduct(ctx context.Context, p Product) (string, error) {
	if p.Slug == "" {
		return "", ErrSlugRequired
	}
	if p.Status == "" {
		p.Status = ProductDraft
	}
	if p.Status != ProductDraft {
		return "", ErrInvalidStatus
	}
	return s.store.CreateProduct(ctx, p)
}

// GetProductDetail returns a product with its variants, or ErrProductNotFound.
func (s *Service) GetProductDetail(ctx context.Context, id string) (*ProductDetail, error) {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProductNotFound
	}
	variants, err := s.store.ListVariantsByProduct(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	return &ProductDetail{Product: *p, Variants: variants}, nil
}

// ListPublishedByCategory returns PUBLISHED products attached to a category, for the
// storefront. page is 1-based.
func (s *Service) ListPublishedByCategory(ctx context.Context, categoryID string, page, pageSize int) ([]Product, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.ListProductsByCategory(ctx, categoryID, ProductPublished, page, pageSize)
}

// PublishProduct moves a product to PUBLISHED after enforcing the publish gate
// (PRD §۲ MA-1/MA-6): sleep_relevance, spec_source and spec_last_reviewed_at must all
// be present and non-empty. On success it sets status=PUBLISHED and published_at=now.
func (s *Service) PublishProduct(ctx context.Context, id string) error {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrProductNotFound
	}
	if p.SleepRelevance == "" || p.SpecSource == "" || p.SpecLastReviewedAt == nil {
		return ErrPublishGateMissing
	}
	now := s.now()
	return s.store.UpdateProductStatus(ctx, id, ProductPublished, &now)
}

// PauseProduct moves a PUBLISHED product back to PAUSED (e.g. out-of-stock hold). The
// publish gate is not required for pausing (it was satisfied at publish time).
func (s *Service) PauseProduct(ctx context.Context, id string) error {
	p, err := s.store.GetProduct(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrProductNotFound
	}
	if p.Status != ProductPublished && p.Status != ProductApproved {
		return ErrInvalidStatus
	}
	return s.store.UpdateProductStatus(ctx, id, ProductPaused, nil)
}

// CreateVariant persists a sellable variant. SKU must be unique (§۱۵); the Service
// checks for an existing variant with the same SKU before inserting.
func (s *Service) CreateVariant(ctx context.Context, v Variant) (string, error) {
	if v.SKU == "" {
		return "", ErrSlugRequired
	}
	if v.ProductID == "" {
		return "", ErrProductNotFound
	}
	if v.Status == "" {
		v.Status = VariantDraft
	}
	existing, err := s.store.GetVariantBySKU(ctx, v.SKU)
	if err != nil {
		return "", fmt.Errorf("check sku: %w", err)
	}
	if existing != nil {
		return "", ErrDuplicateSKU
	}
	return s.store.CreateVariant(ctx, v)
}
