package catalog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory catalog.Store for unit tests.
type memStore struct {
	mu    sync.Mutex
	cats  map[string]Category
	prods map[string]*memProduct
	vars  map[string]Variant // keyed by id
	bySKU map[string]string  // sku → variant id
	seq   int
}

type memProduct struct {
	Product
	categoryIDs []string
}

func newMemStore() *memStore {
	return &memStore{cats: map[string]Category{}, prods: map[string]*memProduct{}, vars: map[string]Variant{}, bySKU: map[string]string{}}
}

func (m *memStore) CreateCategory(_ context.Context, c Category) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := c.Slug // deterministic, unique by slug CHECK in real DB
	if id == "" {
		id = "cat-" + itoa(m.seq)
	}
	c.ID = id
	m.cats[id] = c
	return id, nil
}

func (m *memStore) ListCategories(_ context.Context) ([]Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Category, 0, len(m.cats))
	for _, c := range m.cats {
		out = append(out, c)
	}
	return out, nil
}

func (m *memStore) CreateProduct(_ context.Context, p Product) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := p.Slug
	if id == "" {
		id = "prod-" + itoa(m.seq)
	}
	p.ID = id
	mp := &memProduct{Product: p, categoryIDs: append([]string(nil), p.CategoryIDs...)}
	m.prods[id] = mp
	return id, nil
}

func (m *memStore) GetProduct(_ context.Context, id string) (*Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.prods[id]
	if !ok {
		return nil, nil
	}
	p := mp.Product
	p.CategoryIDs = append([]string(nil), mp.categoryIDs...)
	return &p, nil
}

func (m *memStore) ListProductsByCategory(_ context.Context, categoryID, status string, page, pageSize int) ([]Product, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []*memProduct
	for _, mp := range m.prods {
		has := false
		for _, cid := range mp.categoryIDs {
			if cid == categoryID {
				has = true
				break
			}
		}
		if !has {
			continue
		}
		if status != "" && mp.Product.Status != status {
			continue
		}
		matched = append(matched, mp)
	}
	total := len(matched)
	offset := (page - 1) * pageSize
	if offset >= total {
		return []Product{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	out := make([]Product, 0, end-offset)
	for _, mp := range matched[offset:end] {
		out = append(out, mp.Product)
	}
	return out, total, nil
}

func (m *memStore) UpdateProductStatus(_ context.Context, id, status string, publishedAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.prods[id]
	if !ok {
		return ErrProductNotFound
	}
	mp.Status = status
	if publishedAt != nil {
		mp.PublishedAt = publishedAt
	}
	return nil
}

func (m *memStore) CreateVariant(_ context.Context, v Variant) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := v.SKU
	if id == "" {
		id = "var-" + itoa(m.seq)
	}
	v.ID = id
	m.vars[id] = v
	m.bySKU[v.SKU] = id
	return id, nil
}

func (m *memStore) ListVariantsByProduct(_ context.Context, productID string) ([]Variant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Variant
	for _, v := range m.vars {
		if v.ProductID == productID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (m *memStore) GetVariantBySKU(_ context.Context, sku string) (*Variant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.bySKU[sku]
	if !ok {
		return nil, nil
	}
	v := m.vars[id]
	return &v, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestCreateCategory_RequiresSlug(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateCategory(context.Background(), Category{Name: "x"}); !errors.Is(err, ErrSlugRequired) {
		t.Fatalf("expected ErrSlugRequired, got %v", err)
	}
}

func TestCategoryTree_BuildsHierarchyAndOrphansAsRoots(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	root, _ := svc.CreateCategory(context.Background(), Category{Slug: "home", Name: "خانه"})
	child, _ := svc.CreateCategory(context.Background(), Category{Slug: "bedding", Name: "رختخواب", ParentID: root})
	svc.CreateCategory(context.Background(), Category{Slug: "pillows", Name: "بالش", ParentID: child})
	// orphan: parent id points at a missing category
	svc.CreateCategory(context.Background(), Category{Slug: "orphan", Name: "یتیم", ParentID: "no-such-parent"})

	tree, err := svc.CategoryTree(context.Background())
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	// Two roots: "home" and the orphan.
	if len(tree) != 2 {
		t.Fatalf("roots=%d want 2", len(tree))
	}
	home := findNode(tree, "home")
	if home == nil {
		t.Fatal("home root missing")
	}
	if len(home.Children) != 1 || home.Children[0].Slug != "bedding" {
		t.Fatalf("home children = %+v", home.Children)
	}
	if len(home.Children[0].Children) != 1 || home.Children[0].Children[0].Slug != "pillows" {
		t.Fatalf("nested pillows missing: %+v", home.Children[0].Children)
	}
	if findNode(tree, "orphan") == nil {
		t.Fatal("orphan must appear as a root")
	}
}

func findNode(tree []CategoryNode, slug string) *CategoryNode {
	for i := range tree {
		if tree[i].Slug == slug {
			return &tree[i]
		}
		if c := findNode(tree[i].Children, slug); c != nil {
			return c
		}
	}
	return nil
}

func TestCreateProduct_OnlyDraftInitial(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateProduct(context.Background(), Product{Slug: "p", Status: ProductPublished}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus for non-draft initial, got %v", err)
	}
	id, err := svc.CreateProduct(context.Background(), Product{Slug: "p", Title: "P"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	p, _ := svc.store.(*memStore).GetProduct(context.Background(), id)
	if p.Status != ProductDraft {
		t.Fatalf("initial status=%s want DRAFT", p.Status)
	}
}

func TestPublishProduct_EnforcesGate(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "pillow", Title: "بالش"})
	// No gate fields → must fail.
	if err := svc.PublishProduct(context.Background(), pid); !errors.Is(err, ErrPublishGateMissing) {
		t.Fatalf("expected ErrPublishGateMissing, got %v", err)
	}
	p, _ := store.GetProduct(context.Background(), pid)
	if p.Status != ProductDraft {
		t.Fatalf("status must remain DRAFT after failed publish, got %s", p.Status)
	}
	// Partial gate (only sleep_relevance) → still fails.
	store.prods[pid].SleepRelevance = "تأثیر خواب"
	if err := svc.PublishProduct(context.Background(), pid); !errors.Is(err, ErrPublishGateMissing) {
		t.Fatalf("partial gate must fail, got %v", err)
	}
	// Full gate → succeeds, status PUBLISHED + published_at set.
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	store.prods[pid].SpecSource = "سازنده"
	reviewed := now.Add(-24 * time.Hour)
	store.prods[pid].SpecLastReviewedAt = &reviewed
	if err := svc.PublishProduct(context.Background(), pid); err != nil {
		t.Fatalf("publish with full gate: %v", err)
	}
	p, _ = store.GetProduct(context.Background(), pid)
	if p.Status != ProductPublished {
		t.Fatalf("status=%s want PUBLISHED", p.Status)
	}
	if p.PublishedAt == nil || !p.PublishedAt.Equal(now) {
		t.Fatalf("published_at=%v want %v", p.PublishedAt, now)
	}
}

func TestPublishProduct_NotFound(t *testing.T) {
	svc := NewService(newMemStore())
	if err := svc.PublishProduct(context.Background(), "nope"); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("expected ErrProductNotFound, got %v", err)
	}
}

func TestPauseProduct_OnlyFromPublishedOrApproved(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "p", Title: "P"})
	// From DRAFT → PAUSE must fail.
	if err := svc.PauseProduct(context.Background(), pid); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("pause from DRAFT should fail, got %v", err)
	}
	// Force to PUBLISHED then pause.
	store.prods[pid].Status = ProductPublished
	if err := svc.PauseProduct(context.Background(), pid); err != nil {
		t.Fatalf("pause from PUBLISHED: %v", err)
	}
	p, _ := store.GetProduct(context.Background(), pid)
	if p.Status != ProductPaused {
		t.Fatalf("status=%s want PAUSED", p.Status)
	}
}

func TestListPublishedByCategory_OnlyPublishedVisible(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	cat, _ := svc.CreateCategory(context.Background(), Category{Slug: "c", Name: "C"})
	// Three products in the category: DRAFT, PUBLISHED, PUBLISHED.
	for i, st := range []string{ProductDraft, ProductPublished, ProductPublished} {
		pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "p" + itoa(i), Title: "P", CategoryIDs: []string{cat}})
		store.prods[pid].Status = st
	}
	prods, total, err := svc.ListPublishedByCategory(context.Background(), cat, 1, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("total=%d want 2 (only PUBLISHED)", total)
	}
	if len(prods) != 2 {
		t.Fatalf("returned=%d want 2", len(prods))
	}
}

func TestListPublishedByCategory_Pagination(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	cat, _ := svc.CreateCategory(context.Background(), Category{Slug: "c", Name: "C"})
	for i := 0; i < 5; i++ {
		pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "p" + itoa(i), Title: "P", CategoryIDs: []string{cat}})
		store.prods[pid].Status = ProductPublished
	}
	// page 1 size 2 → 2 items, total 5; page 3 size 2 → 1 item.
	p1, total, _ := svc.ListPublishedByCategory(context.Background(), cat, 1, 2)
	if total != 5 || len(p1) != 2 {
		t.Fatalf("page1: len=%d total=%d", len(p1), total)
	}
	p3, _, _ := svc.ListPublishedByCategory(context.Background(), cat, 3, 2)
	if len(p3) != 1 {
		t.Fatalf("page3: len=%d want 1", len(p3))
	}
	// page beyond range → empty, total still 5.
	p9, total9, _ := svc.ListPublishedByCategory(context.Background(), cat, 9, 2)
	if len(p9) != 0 || total9 != 5 {
		t.Fatalf("page9: len=%d total=%d want 0/5", len(p9), total9)
	}
}

func TestCreateVariant_EnforcesUniqueSKU(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "p", Title: "P"})
	if _, err := svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: ""}); !errors.Is(err, ErrSlugRequired) {
		t.Fatalf("empty sku: %v", err)
	}
	if _, err := svc.CreateVariant(context.Background(), Variant{ProductID: "", SKU: "s"}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("missing product: %v", err)
	}
	if _, err := svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: "KHA-PIL-001"}); err != nil {
		t.Fatalf("first variant: %v", err)
	}
	if _, err := svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: "KHA-PIL-001"}); !errors.Is(err, ErrDuplicateSKU) {
		t.Fatalf("duplicate sku must fail, got %v", err)
	}
	// Distinct SKU → ok.
	if _, err := svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: "KHA-PIL-002"}); err != nil {
		t.Fatalf("second variant: %v", err)
	}
}

func TestGetProductDetail_IncludesVariants(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	pid, _ := svc.CreateProduct(context.Background(), Product{Slug: "p", Title: "P"})
	svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: "v1"})
	svc.CreateVariant(context.Background(), Variant{ProductID: pid, SKU: "v2"})
	detail, err := svc.GetProductDetail(context.Background(), pid)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.ID != pid {
		t.Fatalf("id=%s want %s", detail.ID, pid)
	}
	if len(detail.Variants) != 2 {
		t.Fatalf("variants=%d want 2", len(detail.Variants))
	}
}

func TestGetProductDetail_NotFound(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.GetProductDetail(context.Background(), "nope"); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("expected ErrProductNotFound, got %v", err)
	}
}
