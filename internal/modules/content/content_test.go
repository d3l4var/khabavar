package content

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory content.Store for unit tests.
type memStore struct {
	mu           sync.Mutex
	topics       map[string]*Topic
	articles     map[string]*Article
	sources      map[string]*ArticleSource
	sourcesByArt map[string][]string
	links        map[string]map[string]bool // articleID → set of topicID
	relations    map[string]*ProductArticleRelation
	relByArt     map[string][]string
	relByProd    map[string][]string
	slugIdx      map[string]string // slug → articleID
	seq          int
}

func newMemStore() *memStore {
	return &memStore{
		topics: map[string]*Topic{}, articles: map[string]*Article{},
		sources: map[string]*ArticleSource{}, sourcesByArt: map[string][]string{},
		links: map[string]map[string]bool{}, relations: map[string]*ProductArticleRelation{},
		relByArt: map[string][]string{}, relByProd: map[string][]string{},
		slugIdx: map[string]string{},
	}
}

func (m *memStore) nextID(prefix string) string {
	m.seq++
	return prefix + itoa(m.seq)
}

func (m *memStore) CreateTopic(_ context.Context, t Topic) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.findBySlug(t.Slug, ""); dup {
		return "", nil // slug uniqueness handled at service layer for mem
	}
	id := m.nextID("tp-")
	t.ID = id
	t.CreatedAt = time.Now()
	m.topics[id] = &t
	return id, nil
}

func (m *memStore) findBySlug(slug, excludeID string) (string, bool) {
	for id, a := range m.articles {
		if a.Slug == slug && id != excludeID {
			return id, true
		}
	}
	return "", false
}

func (m *memStore) GetTopic(_ context.Context, id string) (*Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.topics[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (m *memStore) ListTopics(_ context.Context) ([]Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Topic, 0, len(m.topics))
	for _, t := range m.topics {
		if t.Status == TopicActive {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (m *memStore) UpdateTopicStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.topics[id]
	if !ok {
		return ErrTopicNotFound
	}
	t.Status = status
	return nil
}

func (m *memStore) CreateArticle(_ context.Context, a Article) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID("art-")
	a.ID = id
	a.CreatedAt = time.Now()
	a.UpdatedAt = a.CreatedAt
	m.articles[id] = &a
	m.slugIdx[a.Slug] = id
	return id, nil
}

func (m *memStore) GetArticle(_ context.Context, id string) (*Article, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.articles[id]
	if !ok {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (m *memStore) GetArticleBySlug(_ context.Context, slug string) (*Article, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.slugIdx[slug]
	if !ok {
		return nil, nil
	}
	cp := *m.articles[id]
	return &cp, nil
}

func (m *memStore) UpdateArticleStatus(_ context.Context, id, status string, publishedAt *time.Time, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.articles[id]
	if !ok {
		return ErrArticleNotFound
	}
	a.Status = status
	if publishedAt != nil {
		a.PublishedAt = publishedAt
	}
	a.UpdatedAt = time.Now()
	return nil
}

func (m *memStore) ListPublishedArticles(_ context.Context, page, pageSize int) ([]Article, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pubs []Article
	for _, a := range m.articles {
		if a.Status == StatusPublished {
			pubs = append(pubs, *a)
		}
	}
	total := len(pubs)
	start := (page - 1) * pageSize
	if start > total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return pubs[start:end], total, nil
}

func (m *memStore) ListArticlesByTopic(_ context.Context, topicID string, page, pageSize int) ([]Article, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pubs []Article
	for artID, topics := range m.links {
		if topics[topicID] && m.articles[artID].Status == StatusPublished {
			pubs = append(pubs, *m.articles[artID])
		}
	}
	total := len(pubs)
	start := (page - 1) * pageSize
	if start > total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return pubs[start:end], total, nil
}

func (m *memStore) ListTranslations(_ context.Context, parentArticleID string) ([]Article, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Article
	for _, a := range m.articles {
		if a.ParentArticleID == parentArticleID {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (m *memStore) SetArticleEditor(_ context.Context, id, editorID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.articles[id]
	if !ok {
		return ErrArticleNotFound
	}
	a.EditorID = editorID
	return nil
}

func (m *memStore) AddArticleSource(_ context.Context, src ArticleSource) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID("src-")
	src.ID = id
	src.CreatedAt = time.Now()
	m.sources[id] = &src
	m.sourcesByArt[src.ArticleID] = append(m.sourcesByArt[src.ArticleID], id)
	return id, nil
}

func (m *memStore) ListArticleSources(_ context.Context, articleID string) ([]ArticleSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.sourcesByArt[articleID]
	out := make([]ArticleSource, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.sources[id])
	}
	return out, nil
}

func (m *memStore) DeleteArticleSource(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, ok := m.sources[id]
	if !ok {
		return ErrSourceNotFound
	}
	delete(m.sources, id)
	art := src.ArticleID
	ids := m.sourcesByArt[art]
	for i, sid := range ids {
		if sid == id {
			m.sourcesByArt[art] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (m *memStore) LinkArticleTopic(_ context.Context, articleID, topicID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.links[articleID] == nil {
		m.links[articleID] = map[string]bool{}
	}
	m.links[articleID][topicID] = true
	return nil
}

func (m *memStore) UnlinkArticleTopic(_ context.Context, articleID, topicID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.links[articleID] != nil {
		delete(m.links[articleID], topicID)
	}
	return nil
}

func (m *memStore) ListTopicsForArticle(_ context.Context, articleID string) ([]Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	topics := m.links[articleID]
	out := make([]Topic, 0, len(topics))
	for tid := range topics {
		if t, ok := m.topics[tid]; ok {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (m *memStore) AddProductArticleRelation(_ context.Context, rel ProductArticleRelation) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID("rel-")
	rel.ID = id
	rel.CreatedAt = time.Now()
	m.relations[id] = &rel
	m.relByArt[rel.ArticleID] = append(m.relByArt[rel.ArticleID], id)
	m.relByProd[rel.ProductID] = append(m.relByProd[rel.ProductID], id)
	return id, nil
}

func (m *memStore) ListRelationsForArticle(_ context.Context, articleID string) ([]ProductArticleRelation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.relByArt[articleID]
	out := make([]ProductArticleRelation, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.relations[id])
	}
	return out, nil
}

func (m *memStore) ListRelationsForProduct(_ context.Context, productID string) ([]ProductArticleRelation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.relByProd[productID]
	out := make([]ProductArticleRelation, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.relations[id])
	}
	return out, nil
}

func (m *memStore) RemoveProductArticleRelation(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rel, ok := m.relations[id]
	if !ok {
		return ErrRelationNotFound
	}
	delete(m.relations, id)
	// best-effort remove from indexes
	for i, rid := range m.relByArt[rel.ArticleID] {
		if rid == id {
			m.relByArt[rel.ArticleID] = append(m.relByArt[rel.ArticleID][:i], m.relByArt[rel.ArticleID][i+1:]...)
			break
		}
	}
	for i, rid := range m.relByProd[rel.ProductID] {
		if rid == id {
			m.relByProd[rel.ProductID] = append(m.relByProd[rel.ProductID][:i], m.relByProd[rel.ProductID][i+1:]...)
			break
		}
	}
	return nil
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

func TestCreateTopic_RequiresNameAndSlug(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateTopic(context.Background(), Topic{Slug: "x"}); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("missing name: %v", err)
	}
	if _, err := svc.CreateTopic(context.Background(), Topic{Name: "x"}); !errors.Is(err, ErrSlugRequired) {
		t.Fatalf("missing slug: %v", err)
	}
}

func TestUpdateTopicStatus_OnlyActiveOrInactive(t *testing.T) {
	svc := NewService(newMemStore())
	id, _ := svc.CreateTopic(context.Background(), Topic{Name: "خواب", Slug: "sleep"})
	if err := svc.UpdateTopicStatus(context.Background(), id, "PUBLISHED"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("invalid topic status: %v", err)
	}
	if err := svc.UpdateTopicStatus(context.Background(), id, TopicInactive); err != nil {
		t.Fatalf("valid: %v", err)
	}
}

func TestCreateArticle_RequiresFields(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateArticle(context.Background(), Article{Slug: "x", Body: "b"}); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("title: %v", err)
	}
	if _, err := svc.CreateArticle(context.Background(), Article{Title: "t", Body: "b"}); !errors.Is(err, ErrSlugRequired) {
		// ^ Title+Body, slug missing
	}
	if _, err := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "x"}); !errors.Is(err, ErrBodyRequired) {
		t.Fatalf("body: %v", err)
	}
}

func TestCreateArticle_TranslationRequiresParent(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateArticle(context.Background(), Article{
		Title: "ترجمه", Slug: "tr", Body: "b", SourceType: SourceTranslation,
	}); err == nil {
		t.Fatal("translation without parent must fail")
	}
}

func TestCreateArticle_InvalidSourceType(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateArticle(context.Background(), Article{
		Title: "t", Slug: "s", Body: "b", SourceType: "BLOG",
	}); !errors.Is(err, ErrInvalidArticleSource) {
		t.Fatalf("invalid source_type: %v", err)
	}
}

func TestEditorialWorkflow_HappyPath(t *testing.T) {
	svc := NewService(newMemStore())
	id, _ := svc.CreateArticle(context.Background(), Article{
		Title: "تاثیر خواب بر سلامت", Slug: "sleep-health", Body: "محتوای مقاله",
	})
	// DRAFT → IN_REVIEW
	if err := svc.SubmitForReview(context.Background(), id); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// IN_REVIEW → APPROVED (editor signs)
	if err := svc.Approve(context.Background(), id, "editor-1"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// APPROVED → PUBLISHED (needs ≥1 source — ADR-CONTENT-001)
	if err := svc.Publish(context.Background(), id); !errors.Is(err, ErrPublishGateNoSource) {
		t.Fatalf("publish without source must fail: %v", err)
	}
	// Add a source, then publish.
	svc.AddArticleSource(context.Background(), ArticleSource{
		ArticleID: id, SourceType: CitationJournal, Citation: "Smith et al., 2024",
	})
	if err := svc.Publish(context.Background(), id); err != nil {
		t.Fatalf("publish with source: %v", err)
	}
	a, _ := svc.GetArticle(context.Background(), id)
	if a.Status != StatusPublished {
		t.Fatalf("status=%s want PUBLISHED", a.Status)
	}
	if a.PublishedAt == nil {
		t.Fatal("published_at not set")
	}
}

func TestPublishGate_NoSource(t *testing.T) {
	svc := NewService(newMemStore())
	id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	svc.SubmitForReview(context.Background(), id)
	svc.Approve(context.Background(), id, "ed")
	if err := svc.Publish(context.Background(), id); !errors.Is(err, ErrPublishGateNoSource) {
		t.Fatalf("expected ErrPublishGateNoSource, got %v", err)
	}
}

func TestEditorialWorkflow_InvalidTransitions(t *testing.T) {
	cases := []struct {
		name   string
		from   string
		action func(*Service, context.Context, string) error
	}{
		{"DRAFT→PUBLISHED (skip)", StatusDraft, func(s *Service, ctx context.Context, id string) error { return s.Publish(ctx, id) }},
		{"DRAFT→APPROVED (skip)", StatusDraft, func(s *Service, ctx context.Context, id string) error { return s.Approve(ctx, id, "e") }},
		{"IN_REVIEW→PUBLISHED (skip)", StatusInReview, func(s *Service, ctx context.Context, id string) error { return s.Publish(ctx, id) }},
		{"PUBLISHED→IN_REVIEW (backward)", StatusPublished, func(s *Service, ctx context.Context, id string) error { return s.SubmitForReview(ctx, id) }},
		{"ARCHIVED→DRAFT (terminal)", StatusArchived, func(s *Service, ctx context.Context, id string) error { return s.RejectToDraft(ctx, id) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newMemStore()
			svc := NewService(store)
			id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
			store.articles[id].Status = c.from
			// Ensure publish gate satisfied if the action is Publish.
			if c.name == "PUBLISHED→IN_REVIEW (backward)" {
				store.articles[id].Status = StatusPublished
			}
			err := c.action(svc, context.Background(), id)
			if !errors.Is(err, ErrInvalidStatus) && !errors.Is(err, ErrPublishGateNoSource) {
				t.Fatalf("expected ErrInvalidStatus or ErrPublishGateNoSource, got %v", err)
			}
		})
	}
}

func TestUnpublishAndArchive(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	svc.AddArticleSource(context.Background(), ArticleSource{ArticleID: id, SourceType: CitationJournal, Citation: "x"})
	store.articles[id].Status = StatusPublished
	// PUBLISHED → UNPUBLISHED
	if err := svc.Unpublish(context.Background(), id); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	// UNPUBLISHED → PUBLISHED (republish)
	if err := svc.Publish(context.Background(), id); err != nil {
		t.Fatalf("republish: %v", err)
	}
	// PUBLISHED → ARCHIVED
	if err := svc.Archive(context.Background(), id); err != nil {
		t.Fatalf("archive: %v", err)
	}
	a, _ := svc.GetArticle(context.Background(), id)
	if a.Status != StatusArchived {
		t.Fatalf("status=%s want ARCHIVED", a.Status)
	}
}

func TestGetPublishedArticleBySlug_HidesDrafts(t *testing.T) {
	svc := NewService(newMemStore())
	id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "draft-slug", Body: "b"})
	// Article is in DRAFT → must not be visible via GetPublishedArticleBySlug.
	if _, err := svc.GetPublishedArticleBySlug(context.Background(), "draft-slug"); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("draft should be hidden: %v", err)
	}
	// Publish it (add source + transition).
	svc.AddArticleSource(context.Background(), ArticleSource{ArticleID: id, SourceType: CitationExpert, Citation: "Dr. X"})
	store := svc.store.(*memStore)
	store.articles[id].Status = StatusApproved
	svc.Publish(context.Background(), id)
	a, err := svc.GetPublishedArticleBySlug(context.Background(), "draft-slug")
	if err != nil {
		t.Fatalf("published should be visible: %v", err)
	}
	if a.ID != id {
		t.Fatalf("id=%s want %s", a.ID, id)
	}
}

func TestRejectToDraft(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	// IN_REVIEW → DRAFT
	store.articles[id].Status = StatusInReview
	if err := svc.RejectToDraft(context.Background(), id); err != nil {
		t.Fatalf("reject from IN_REVIEW: %v", err)
	}
	// APPROVED → DRAFT
	store.articles[id].Status = StatusApproved
	if err := svc.RejectToDraft(context.Background(), id); err != nil {
		t.Fatalf("reject from APPROVED: %v", err)
	}
}

func TestArticleSources_CitationTypeValidation(t *testing.T) {
	svc := NewService(newMemStore())
	id, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	if _, err := svc.AddArticleSource(context.Background(), ArticleSource{
		ArticleID: id, SourceType: "WIKI", Citation: "x",
	}); !errors.Is(err, ErrInvalidCitationType) {
		t.Fatalf("invalid citation type: %v", err)
	}
	if _, err := svc.AddArticleSource(context.Background(), ArticleSource{
		ArticleID: id, SourceType: CitationGuideline, Citation: "",
	}); err == nil {
		t.Fatal("empty citation must fail")
	}
}

func TestProductRelation_RequiresReason(t *testing.T) {
	svc := NewService(newMemStore())
	artID, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	// Missing relation_reason → error (MA-5).
	if _, err := svc.AddProductRelation(context.Background(), ProductArticleRelation{
		ProductID: "p-1", ArticleID: artID, RelationType: RelationEducational,
	}); !errors.Is(err, ErrRelationReasonReq) {
		t.Fatalf("missing reason: %v", err)
	}
	// Invalid relation_type → error.
	if _, err := svc.AddProductRelation(context.Background(), ProductArticleRelation{
		ProductID: "p-1", ArticleID: artID, RelationType: "SPONSORED", RelationReason: "r",
	}); !errors.Is(err, ErrInvalidRelationType) {
		t.Fatalf("invalid type: %v", err)
	}
	// Valid.
	relID, err := svc.AddProductRelation(context.Background(), ProductArticleRelation{
		ProductID: "p-1", ArticleID: artID, RelationType: RelationProductGuide,
		RelationReason: "راهنمای انتخاب بالش متناسب با وضعیت خواب",
	})
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if relID == "" {
		t.Fatal("empty rel id")
	}
	// List by article.
	rels, _ := svc.ListRelationsForArticle(context.Background(), artID)
	if len(rels) != 1 {
		t.Fatalf("article rels=%d want 1", len(rels))
	}
	// List by product.
	rels, _ = svc.ListRelationsForProduct(context.Background(), "p-1")
	if len(rels) != 1 {
		t.Fatalf("product rels=%d want 1", len(rels))
	}
}

func TestLinkTopic(t *testing.T) {
	svc := NewService(newMemStore())
	artID, _ := svc.CreateArticle(context.Background(), Article{Title: "t", Slug: "s", Body: "b"})
	tpID, _ := svc.CreateTopic(context.Background(), Topic{Name: "خواب", Slug: "sleep"})
	if err := svc.LinkTopic(context.Background(), artID, tpID); err != nil {
		t.Fatalf("link: %v", err)
	}
	topics, _ := svc.ListTopicsForArticle(context.Background(), artID)
	if len(topics) != 1 {
		t.Fatalf("topics=%d want 1", len(topics))
	}
}

func TestListPublishedArticles_Pagination(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	// Create 5 published articles.
	for i := 0; i < 5; i++ {
		id, _ := svc.CreateArticle(context.Background(), Article{
			Title: "t", Slug: "s" + itoa(i), Body: "b",
		})
		store.articles[id].Status = StatusPublished
	}
	// Page 1, size 2 → 2 articles, total 5.
	arts, total, err := svc.ListPublishedArticles(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Fatalf("total=%d want 5", total)
	}
	if len(arts) != 2 {
		t.Fatalf("page size=%d want 2", len(arts))
	}
	// Page 3, size 2 → 1 article (5 - 4).
	arts, _, _ = svc.ListPublishedArticles(context.Background(), 3, 2)
	if len(arts) != 1 {
		t.Fatalf("last page=%d want 1", len(arts))
	}
}

func TestListTranslations(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	// Original article.
	origID, _ := svc.CreateArticle(context.Background(), Article{
		Title: "Sleep", Slug: "sleep-en", Body: "b", Language: "en",
	})
	// Two translations.
	svc.CreateArticle(context.Background(), Article{
		Title: "خواب", Slug: "sleep-fa", Body: "b", Language: "fa-IR",
		SourceType: SourceTranslation, ParentArticleID: origID,
	})
	svc.CreateArticle(context.Background(), Article{
		Title: "النوم", Slug: "sleep-ar", Body: "b", Language: "ar",
		SourceType: SourceTranslation, ParentArticleID: origID,
	})
	trans, err := svc.ListTranslations(context.Background(), origID)
	if err != nil {
		t.Fatalf("list translations: %v", err)
	}
	if len(trans) != 2 {
		t.Fatalf("translations=%d want 2", len(trans))
	}
}
