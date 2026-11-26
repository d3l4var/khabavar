// Package content implements the content/magazine module (SAD §۷.۲, DB v2 §۱۲,
// R-04 §۴, ADR-CONTENT-001): topics (category tree), articles with editorial
// workflow, article sources (the publish gate), product-article relations
// (§12.3 with a required relation_reason per MA-5), and language-variant
// translations (parent_article_id chain, SAD §۷.۲).
//
// The signature business rules enforced here:
//   - Editorial workflow (SAD §۷.۲): DRAFT → IN_REVIEW → APPROVED → SCHEDULED →
//     PUBLISHED → UNPUBLISHED → ARCHIVED. Each transition has an allowed-from
//     set; backward transitions (e.g. PUBLISHED → IN_REVIEW) are rejected.
//   - Publish gate (ADR-CONTENT-001): an article CANNOT be published without
//     at least one article_source. The Service checks this before
//     DRAFT/IN_REVIEW/APPROVED → PUBLISHED. This prevents publishing
//     unsubstantiated health claims.
//   - Slug uniqueness (§۱۵): article slugs and topic slugs are unique.
//   - Product-article relation reason (MA-5): every product_article_relation
//     MUST carry a relation_reason — "related" is not enough; the editorial
//     reason is recorded (educational context, product guide, comparison).
//   - Translation chain (SAD §۷.۲): a translation article sets parent_article_id
//     to the original; the original's source_type is ORIGINAL, translations are
//     TRANSLATION. This forms a language-variant family.
package content

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status / type enums (mirrors CHECK constraints in 000008_content.up.sql).
const (
	// Article status (SAD §۷.۲ editorial workflow).
	StatusDraft       = "DRAFT"
	StatusInReview    = "IN_REVIEW"
	StatusApproved    = "APPROVED"
	StatusScheduled   = "SCHEDULED"
	StatusPublished   = "PUBLISHED"
	StatusUnpublished = "UNPUBLISHED"
	StatusArchived    = "ARCHIVED"

	// Article source_type (SAD §۷.۳).
	SourceOriginal    = "ORIGINAL"
	SourceTranslation = "TRANSLATION"
	SourceCurated     = "CURATED"

	// Article citation source_type (ADR-CONTENT-001).
	CitationJournal   = "JOURNAL"
	CitationBook      = "BOOK"
	CitationGuideline = "GUIDELINE"
	CitationExpert    = "EXPERT"
	CitationOther     = "OTHER"

	// Topic status.
	TopicActive   = "ACTIVE"
	TopicInactive = "INACTIVE"

	// Product-article relation type (§12.3).
	RelationRelated      = "RELATED"
	RelationEducational  = "EDUCATIONAL"
	RelationProductGuide = "PRODUCT_GUIDE"
	RelationComparison   = "COMPARISON"

	DefaultLanguage = "fa-IR"
)

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrTopicNotFound        = errors.New("content: topic not found")
	ErrArticleNotFound      = errors.New("content: article not found")
	ErrSourceNotFound       = errors.New("content: article source not found")
	ErrRelationNotFound     = errors.New("content: product-article relation not found")
	ErrSlugRequired         = errors.New("content: slug required")
	ErrDuplicateSlug        = errors.New("content: slug already exists")
	ErrTitleRequired        = errors.New("content: title required")
	ErrBodyRequired         = errors.New("content: body required")
	ErrInvalidStatus        = errors.New("content: invalid status transition")
	ErrPublishGateNoSource  = errors.New("content: cannot publish without at least one source (ADR-CONTENT-001)")
	ErrRelationReasonReq    = errors.New("content: relation_reason required (MA-5)")
	ErrInvalidRelationType  = errors.New("content: invalid relation_type")
	ErrInvalidCitationType  = errors.New("content: invalid citation source_type")
	ErrInvalidArticleSource = errors.New("content: invalid article source_type")
	ErrCannotPublish        = errors.New("content: article is not in a publishable status")
)

// Topic mirrors topics (DB v2 §۱۲.۱). A topic tree via parent_id.
type Topic struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	ParentID  string    `json:"parent_id,omitempty"`
	SortOrder int       `json:"sort_order"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Article mirrors articles (DB v2 §۱۲.۱). The editorial workflow status is the
// core state; published_at is set on first PUBLISHED transition.
type Article struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Slug            string     `json:"slug"`
	Summary         string     `json:"summary"`
	Body            string     `json:"body"`
	Language        string     `json:"language"`
	SourceType      string     `json:"source_type"`
	SourceURL       string     `json:"source_url,omitempty"`
	ParentArticleID string     // set for translations (SAD §۷.۲) `json:"parent_article_id,omitempty"`
	AuthorID        string     `json:"author_id,omitempty"`
	EditorID        string     `json:"editor_id,omitempty"`
	Status          string     `json:"status"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ArticleSource mirrors article_sources (ADR-CONTENT-001). An article must have
// ≥1 source to be publishable.
type ArticleSource struct {
	ID         string    `json:"id"`
	ArticleID  string    `json:"article_id"`
	SourceType string    // JOURNAL/BOOK/GUIDELINE/EXPERT/OTHER `json:"source_type"`
	Citation   string    `json:"citation"`
	URL        string    `json:"url,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ProductArticleRelation mirrors product_article_relations (§12.3). The
// relation_reason is mandatory (MA-5).
type ProductArticleRelation struct {
	ID             string
	ProductID      string
	ArticleID      string
	RelationType   string // RELATED/EDUCATIONAL/PRODUCT_GUIDE/COMPARISON
	RelationReason string
	CreatedAt      time.Time
}

// Store is the persistence boundary for content. The DB implementation uses
// pgx; unit tests use an in-memory implementation.
type Store interface {
	// Topics
	CreateTopic(ctx context.Context, t Topic) (string, error)
	GetTopic(ctx context.Context, id string) (*Topic, error)
	ListTopics(ctx context.Context) ([]Topic, error)
	UpdateTopicStatus(ctx context.Context, id, status string) error

	// Articles
	CreateArticle(ctx context.Context, a Article) (string, error)
	GetArticle(ctx context.Context, id string) (*Article, error)
	GetArticleBySlug(ctx context.Context, slug string) (*Article, error)
	UpdateArticleStatus(ctx context.Context, id, status string, publishedAt *time.Time, now time.Time) error
	UpdateArticle(ctx context.Context, a Article) error
	ListPublishedArticles(ctx context.Context, page, pageSize int) ([]Article, int, error)
	ListArticlesByTopic(ctx context.Context, topicID string, page, pageSize int) ([]Article, int, error)
	ListTranslations(ctx context.Context, parentArticleID string) ([]Article, error)
	SetArticleEditor(ctx context.Context, id, editorID string) error

	// Article sources
	AddArticleSource(ctx context.Context, src ArticleSource) (string, error)
	ListArticleSources(ctx context.Context, articleID string) ([]ArticleSource, error)
	DeleteArticleSource(ctx context.Context, id string) error

	// Article-topic links
	LinkArticleTopic(ctx context.Context, articleID, topicID string) error
	UnlinkArticleTopic(ctx context.Context, articleID, topicID string) error
	ListTopicsForArticle(ctx context.Context, articleID string) ([]Topic, error)

	// Product-article relations
	AddProductArticleRelation(ctx context.Context, rel ProductArticleRelation) (string, error)
	ListRelationsForArticle(ctx context.Context, articleID string) ([]ProductArticleRelation, error)
	ListRelationsForProduct(ctx context.Context, productID string) ([]ProductArticleRelation, error)
	RemoveProductArticleRelation(ctx context.Context, id string) error
}

// Service is the content module boundary (SAD §۴.۲).
type Service struct {
	store  Store
	search ArticleSearchStore // optional; nil → bounded in-memory scan (tests only)
	now    func() time.Time
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

// ── Topics ────────────────────────────────────────────────────────────────────

// CreateTopic creates a new topic (magazine category). parent_id may be empty
// for a root topic.
func (s *Service) CreateTopic(ctx context.Context, t Topic) (string, error) {
	if t.Name == "" {
		return "", ErrTitleRequired
	}
	if t.Slug == "" {
		return "", ErrSlugRequired
	}
	if t.Status == "" {
		t.Status = TopicActive
	}
	return s.store.CreateTopic(ctx, t)
}

// ListTopics returns the full topic list (the frontend builds the tree).
func (s *Service) ListTopics(ctx context.Context) ([]Topic, error) {
	return s.store.ListTopics(ctx)
}

// UpdateTopicStatus toggles ACTIVE/INACTIVE.
func (s *Service) UpdateTopicStatus(ctx context.Context, id, status string) error {
	if status != TopicActive && status != TopicInactive {
		return fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}
	return s.store.UpdateTopicStatus(ctx, id, status)
}

// ── Articles ──────────────────────────────────────────────────────────────────

// CreateArticle creates a new article. status defaults to DRAFT. For translations,
// source_type=TRANSLATION and parent_article_id must point to the original.
func (s *Service) CreateArticle(ctx context.Context, a Article) (string, error) {
	if a.Title == "" {
		return "", ErrTitleRequired
	}
	if a.Slug == "" {
		return "", ErrSlugRequired
	}
	if a.Body == "" {
		return "", ErrBodyRequired
	}
	if a.Language == "" {
		a.Language = DefaultLanguage
	}
	if a.SourceType == "" {
		a.SourceType = SourceOriginal
	}
	if !validArticleSourceType(a.SourceType) {
		return "", ErrInvalidArticleSource
	}
	// Translations must have a parent.
	if a.SourceType == SourceTranslation && a.ParentArticleID == "" {
		return "", errors.New("content: translation requires parent_article_id")
	}
	if a.Status == "" {
		a.Status = StatusDraft
	}
	return s.store.CreateArticle(ctx, a)
}

func validArticleSourceType(t string) bool {
	return t == SourceOriginal || t == SourceTranslation || t == SourceCurated
}

// GetArticle returns an article by ID (admin/editor view; sees all statuses).
func (s *Service) GetArticle(ctx context.Context, id string) (*Article, error) {
	if id == "" {
		return nil, ErrArticleNotFound
	}
	a, err := s.store.GetArticle(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrArticleNotFound
	}
	return a, nil
}

// GetPublishedArticleBySlug returns a PUBLISHED article by slug (storefront).
// Non-published articles return ErrArticleNotFound (the storefront must not
// leak drafts).
func (s *Service) GetPublishedArticleBySlug(ctx context.Context, slug string) (*Article, error) {
	if slug == "" {
		return nil, ErrArticleNotFound
	}
	a, err := s.store.GetArticleBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if a == nil || a.Status != StatusPublished {
		return nil, ErrArticleNotFound
	}
	return a, nil
}

// allowedTransitions defines the editorial workflow state machine (SAD §۷.۲).
var allowedTransitions = map[string][]string{
	StatusDraft:       {StatusInReview},
	StatusInReview:    {StatusApproved, StatusDraft},
	StatusApproved:    {StatusScheduled, StatusPublished, StatusDraft},
	StatusScheduled:   {StatusPublished, StatusApproved},
	StatusPublished:   {StatusUnpublished, StatusArchived},
	StatusUnpublished: {StatusPublished, StatusArchived},
	StatusArchived:    {}, // terminal
}

// SubmitForReview moves DRAFT → IN_REVIEW.
func (s *Service) SubmitForReview(ctx context.Context, id string) error {
	return s.transition(ctx, id, StatusInReview, []string{StatusDraft})
}

// Approve moves IN_REVIEW → APPROVED (editor signs off).
func (s *Service) Approve(ctx context.Context, id, editorID string) error {
	if err := s.transition(ctx, id, StatusApproved, []string{StatusInReview}); err != nil {
		return err
	}
	return s.store.SetArticleEditor(ctx, id, editorID)
}

// Schedule moves APPROVED → SCHEDULED (publish_at set later by a separate call).
func (s *Service) Schedule(ctx context.Context, id string) error {
	return s.transition(ctx, id, StatusScheduled, []string{StatusApproved})
}

// Publish moves APPROVED/SCHEDULED/UNPUBLISHED → PUBLISHED. Enforces the
// ADR-CONTENT-001 publish gate: at least one article_source must exist.
func (s *Service) Publish(ctx context.Context, id string) error {
	// Check the publish gate first (ADR-CONTENT-001).
	sources, err := s.store.ListArticleSources(ctx, id)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}
	if len(sources) == 0 {
		return ErrPublishGateNoSource
	}
	now := s.now()
	if err := s.transitionWithPublish(ctx, id, StatusPublished,
		[]string{StatusApproved, StatusScheduled, StatusUnpublished}, &now); err != nil {
		return err
	}
	return nil
}

// Unpublish moves PUBLISHED → UNPUBLISHED.
func (s *Service) Unpublish(ctx context.Context, id string) error {
	return s.transition(ctx, id, StatusUnpublished, []string{StatusPublished})
}

// Archive moves PUBLISHED/UNPUBLISHED → ARCHIVED (terminal).
func (s *Service) Archive(ctx context.Context, id string) error {
	return s.transition(ctx, id, StatusArchived, []string{StatusPublished, StatusUnpublished})
}

// RejectToDraft moves IN_REVIEW/APPROVED → DRAFT (editor sends back for revision).
func (s *Service) RejectToDraft(ctx context.Context, id string) error {
	return s.transition(ctx, id, StatusDraft, []string{StatusInReview, StatusApproved})
}

// transition is the shared status-transition guard for transitions that don't
// set published_at.
func (s *Service) transition(ctx context.Context, id, to string, allowedFrom []string) error {
	a, err := s.store.GetArticle(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrArticleNotFound
	}
	if !statusInList(a.Status, allowedFrom) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidStatus, a.Status, to)
	}
	return s.store.UpdateArticleStatus(ctx, id, to, nil, s.now())
}

// transitionWithPublish is like transition but sets published_at (for PUBLISHED).
func (s *Service) transitionWithPublish(ctx context.Context, id, to string, allowedFrom []string, publishedAt *time.Time) error {
	a, err := s.store.GetArticle(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrArticleNotFound
	}
	if !statusInList(a.Status, allowedFrom) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidStatus, a.Status, to)
	}
	return s.store.UpdateArticleStatus(ctx, id, to, publishedAt, s.now())
}

func statusInList(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ListPublishedArticles returns paginated PUBLISHED articles (storefront home).
func (s *Service) ListPublishedArticles(ctx context.Context, page, pageSize int) ([]Article, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.ListPublishedArticles(ctx, page, pageSize)
}

// ListArticlesByTopic returns paginated PUBLISHED articles in a topic.
func (s *Service) ListArticlesByTopic(ctx context.Context, topicID string, page, pageSize int) ([]Article, int, error) {
	if topicID == "" {
		return nil, 0, ErrTopicNotFound
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.ListArticlesByTopic(ctx, topicID, page, pageSize)
}

// ListTranslations returns the language-variant family of an article.
func (s *Service) ListTranslations(ctx context.Context, parentArticleID string) ([]Article, error) {
	if parentArticleID == "" {
		return nil, ErrArticleNotFound
	}
	return s.store.ListTranslations(ctx, parentArticleID)
}

// ── Article sources ───────────────────────────────────────────────────────────

// AddArticleSource adds a citation source to an article. This is the gate for
// publishing (ADR-CONTENT-001): an article with no sources cannot be published.
func (s *Service) AddArticleSource(ctx context.Context, src ArticleSource) (string, error) {
	if src.ArticleID == "" {
		return "", ErrArticleNotFound
	}
	if !validCitationType(src.SourceType) {
		return "", ErrInvalidCitationType
	}
	if src.Citation == "" {
		return "", errors.New("content: citation required")
	}
	return s.store.AddArticleSource(ctx, src)
}

func validCitationType(t string) bool {
	return t == CitationJournal || t == CitationBook || t == CitationGuideline ||
		t == CitationExpert || t == CitationOther
}

// ListArticleSources returns all citation sources for an article.
func (s *Service) ListArticleSources(ctx context.Context, articleID string) ([]ArticleSource, error) {
	if articleID == "" {
		return nil, ErrArticleNotFound
	}
	return s.store.ListArticleSources(ctx, articleID)
}

// DeleteArticleSource removes a citation source. If this was the last source
// and the article is PUBLISHED, the article stays published (the gate is only
// checked at publish time, not retroactively — ADR-CONTENT-001 §۳).
func (s *Service) DeleteArticleSource(ctx context.Context, id string) error {
	if id == "" {
		return ErrSourceNotFound
	}
	return s.store.DeleteArticleSource(ctx, id)
}

// ── Article-topic links ───────────────────────────────────────────────────────

// LinkTopic associates an article with a topic.
func (s *Service) LinkTopic(ctx context.Context, articleID, topicID string) error {
	if articleID == "" {
		return ErrArticleNotFound
	}
	if topicID == "" {
		return ErrTopicNotFound
	}
	return s.store.LinkArticleTopic(ctx, articleID, topicID)
}

// ListTopicsForArticle returns the topics an article belongs to.
func (s *Service) ListTopicsForArticle(ctx context.Context, articleID string) ([]Topic, error) {
	if articleID == "" {
		return nil, ErrArticleNotFound
	}
	return s.store.ListTopicsForArticle(ctx, articleID)
}

// ── Product-article relations ─────────────────────────────────────────────────

// AddProductRelation links a product to an article with a mandatory
// relation_reason (MA-5: the editorial reason is recorded, not just "related").
func (s *Service) AddProductRelation(ctx context.Context, rel ProductArticleRelation) (string, error) {
	if rel.ProductID == "" || rel.ArticleID == "" {
		return "", ErrRelationNotFound
	}
	if !validRelationType(rel.RelationType) {
		return "", ErrInvalidRelationType
	}
	if rel.RelationReason == "" {
		return "", ErrRelationReasonReq
	}
	return s.store.AddProductArticleRelation(ctx, rel)
}

func validRelationType(t string) bool {
	return t == RelationRelated || t == RelationEducational ||
		t == RelationProductGuide || t == RelationComparison
}

// ListRelationsForArticle returns all product links for an article (storefront
// "related products" section under an article).
func (s *Service) ListRelationsForArticle(ctx context.Context, articleID string) ([]ProductArticleRelation, error) {
	if articleID == "" {
		return nil, ErrArticleNotFound
	}
	return s.store.ListRelationsForArticle(ctx, articleID)
}

// ListRelationsForProduct returns all article links for a product (storefront
// "related articles" section on a product page — MA-5).
func (s *Service) ListRelationsForProduct(ctx context.Context, productID string) ([]ProductArticleRelation, error) {
	if productID == "" {
		return nil, ErrRelationNotFound
	}
	return s.store.ListRelationsForProduct(ctx, productID)
}

// ArticlePatch carries the editable fields; a nil pointer means "leave it alone", so a
// PATCH never blanks a field the caller did not mention.
type ArticlePatch struct {
	Title    *string
	Slug     *string
	Summary  *string
	Body     *string
	Language *string
}

// UpdateArticle edits an article. A PUBLISHED article may still be edited, but it must
// keep a title, a slug and a body — blanking one would break the public page.
func (s *Service) UpdateArticle(ctx context.Context, id string, patch ArticlePatch) (*Article, error) {
	a, err := s.store.GetArticle(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrArticleNotFound
	}
	if patch.Title != nil {
		a.Title = *patch.Title
	}
	if patch.Slug != nil {
		if *patch.Slug == "" {
			return nil, ErrSlugRequired
		}
		a.Slug = *patch.Slug
	}
	if patch.Summary != nil {
		a.Summary = *patch.Summary
	}
	if patch.Body != nil {
		a.Body = *patch.Body
	}
	if patch.Language != nil {
		a.Language = *patch.Language
	}
	if a.Title == "" {
		return nil, ErrTitleRequired
	}
	if a.Body == "" {
		return nil, ErrBodyRequired
	}
	if err := s.store.UpdateArticle(ctx, *a); err != nil {
		return nil, err
	}
	return a, nil
}
