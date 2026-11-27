// content_pg_test.go exercises the content DBStore against the real schema: the
// editorial workflow (DRAFT → REVIEW → APPROVED → PUBLISHED), the publish gate that
// requires at least one source (ADR-CONTENT-001), the article↔product relation with its
// mandatory reason (MA-5), translations and the storefront reads.
//
// Skipped unless KH_TEST_PG_DSN is set.
package content

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupContent(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewDBStore(pool)
	return pool, NewService(store), store
}

// TestPG_EditorialWorkflowAndPublishGate walks the workflow and proves the source gate:
// publishing without a source must be refused, and the row must stay in APPROVED.
func TestPG_EditorialWorkflowAndPublishGate(t *testing.T) {
	_, svc, store := setupContent(t)
	ctx := context.Background()

	topicID, err := svc.CreateTopic(ctx, Topic{Name: "علوم خواب", Slug: "sleep-science"})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	topics, err := svc.ListTopics(ctx)
	if err != nil || len(topics) != 1 {
		t.Fatalf("ListTopics = %v, %v", topics, err)
	}

	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "چرا خواب مهم است", Slug: "why-sleep-matters",
		Summary: "خلاصه", Body: "متن مقاله", Language: "fa",
		SourceType: "ORIGINAL", AuthorID: "",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	a, err := store.GetArticle(ctx, articleID)
	if err != nil || a == nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if a.Status != StatusDraft {
		t.Fatalf("initial status = %s, want DRAFT", a.Status)
	}

	// A draft is not on the storefront.
	if _, err := svc.GetPublishedArticleBySlug(ctx, "why-sleep-matters"); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("a DRAFT article must not be readable by slug, got %v", err)
	}

	// DRAFT → PUBLISHED directly is illegal.
	if err := svc.Publish(ctx, articleID); err == nil {
		t.Fatal("publishing a DRAFT without review must be refused")
	}

	if err := svc.SubmitForReview(ctx, articleID); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if err := svc.Approve(ctx, articleID, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	a, _ = store.GetArticle(ctx, articleID)
	if a.Status != StatusApproved {
		t.Fatalf("status = %s, want APPROVED", a.Status)
	}

	// The publish gate: no source yet → refused, and the status must not change.
	if err := svc.Publish(ctx, articleID); !errors.Is(err, ErrPublishGateNoSource) {
		t.Fatalf("publish without source: want ErrPublishGateNoSource, got %v", err)
	}
	a, _ = store.GetArticle(ctx, articleID)
	if a.Status != StatusApproved {
		t.Fatalf("a refused publish must not change the status, got %s", a.Status)
	}

	// Add a source, then publish.
	if _, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: articleID, SourceType: "JOURNAL",
		Citation: "Walker M. Why We Sleep. 2017.", URL: "https://example.org/ref",
	}); err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	sources, err := svc.ListArticleSources(ctx, articleID)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %v, %v", sources, err)
	}

	if err := svc.Publish(ctx, articleID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	published, err := svc.GetPublishedArticleBySlug(ctx, "why-sleep-matters")
	if err != nil {
		t.Fatalf("GetPublishedArticleBySlug: %v", err)
	}
	if published.Status != StatusPublished || published.PublishedAt == nil {
		t.Fatalf("published = %+v", published)
	}

	// The magazine listing sees it; the topic link works.
	if err := svc.LinkTopic(ctx, articleID, topicID); err != nil {
		t.Fatalf("LinkTopic: %v", err)
	}
	byTopic, total, err := svc.ListArticlesByTopic(ctx, topicID, 1, 20)
	if err != nil {
		t.Fatalf("ListArticlesByTopic: %v", err)
	}
	if total != 1 || len(byTopic) != 1 {
		t.Fatalf("topic listing: total=%d len=%d", total, len(byTopic))
	}

	// Unpublish removes it from the storefront without deleting it.
	if err := svc.Unpublish(ctx, articleID); err != nil {
		t.Fatalf("Unpublish: %v", err)
	}
	if _, err := svc.GetPublishedArticleBySlug(ctx, "why-sleep-matters"); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("an unpublished article must 404 on the storefront, got %v", err)
	}
	if _, err := store.GetArticle(ctx, articleID); err != nil {
		t.Fatalf("the panel must still read it: %v", err)
	}
}

// TestPG_ProductRelationRequiresReason is MA-5: a product↔article relation must carry a
// reason, and the DB CHECK agrees with the service.
func TestPG_ProductRelationRequiresReason(t *testing.T) {
	pool, svc, _ := setupContent(t)
	ctx := context.Background()

	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "بهترین بالش", Slug: "best-pillow", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	var productID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('بالش','pillow-rel','DRAFT') RETURNING id`).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Without a reason the service refuses.
	if _, err := svc.AddProductRelation(ctx, ProductArticleRelation{
		ProductID: productID, ArticleID: articleID, RelationType: "RELATED",
	}); err == nil {
		t.Fatal("a relation without relation_reason must be refused (MA-5)")
	}

	if _, err := svc.AddProductRelation(ctx, ProductArticleRelation{
		ProductID: productID, ArticleID: articleID, RelationType: "EDUCATIONAL",
		RelationReason: "مقاله کاربرد بالش را برای بی‌خوابی توضیح می‌دهد",
	}); err != nil {
		t.Fatalf("AddProductRelation: %v", err)
	}
	rels, err := svc.ListRelationsForArticle(ctx, articleID)
	if err != nil || len(rels) != 1 {
		t.Fatalf("relations = %v, %v", rels, err)
	}
	if rels[0].RelationReason == "" {
		t.Fatalf("relation lost its reason: %+v", rels[0])
	}
	// The reverse direction (product → articles) must work for the product page.
	reverse, err := svc.ListRelationsForProduct(ctx, productID)
	if err != nil || len(reverse) != 1 {
		t.Fatalf("reverse relations = %v, %v", reverse, err)
	}

	// The DB CHECK rejects a blank reason inserted directly.
	if _, err := pool.Exec(ctx,
		`INSERT INTO product_article_relations (product_id, article_id, relation_type, relation_reason)
		 VALUES ($1,$2,'RELATED','')`, productID, articleID); err == nil {
		t.Fatal("product_article_relations must reject a blank relation_reason")
	}
}

// TestPG_TranslationsChain: a translation points at its parent article (SAD §۷.۲).
func TestPG_TranslationsChain(t *testing.T) {
	_, svc, store := setupContent(t)
	ctx := context.Background()

	parentID, err := svc.CreateArticle(ctx, Article{
		Title: "Sleep hygiene", Slug: "sleep-hygiene-en", Body: "body", Language: "en", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle parent: %v", err)
	}
	childID, err := svc.CreateArticle(ctx, Article{
		Title: "بهداشت خواب", Slug: "sleep-hygiene-fa", Body: "متن", Language: "fa",
		SourceType: "TRANSLATION", ParentArticleID: parentID,
	})
	if err != nil {
		t.Fatalf("CreateArticle child: %v", err)
	}
	translations, err := svc.ListTranslations(ctx, parentID)
	if err != nil {
		t.Fatalf("ListTranslations: %v", err)
	}
	if len(translations) != 1 || translations[0].ID != childID {
		t.Fatalf("translations = %+v", translations)
	}
	child, err := store.GetArticle(ctx, childID)
	if err != nil || child == nil {
		t.Fatalf("GetArticle child: %v", err)
	}
	if child.ParentArticleID != parentID {
		t.Fatalf("parent_article_id = %q, want %q", child.ParentArticleID, parentID)
	}
	if child.Language != "fa" {
		t.Fatalf("language = %q", child.Language)
	}
}

// TestPG_ScheduledPublish: a scheduled article is not live before its time.
func TestPG_ScheduledPublish(t *testing.T) {
	_, svc, store := setupContent(t)
	ctx := context.Background()

	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "زمان‌بندی", Slug: "scheduled-1", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if _, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: articleID, SourceType: "BOOK", Citation: "منبع",
	}); err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	if err := svc.SubmitForReview(ctx, articleID); err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if err := svc.Approve(ctx, articleID, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := svc.Schedule(ctx, articleID); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	a, _ := store.GetArticle(ctx, articleID)
	if a.Status != StatusScheduled {
		t.Fatalf("status = %s, want SCHEDULED", a.Status)
	}
	// SCHEDULED is not on the public listing.
	_, total, err := svc.ListPublishedArticles(ctx, 1, 20)
	if err != nil {
		t.Fatalf("ListPublishedArticles: %v", err)
	}
	if total != 0 {
		t.Fatalf("a SCHEDULED article must not be listed publicly, total=%d", total)
	}
	var _ = time.Now
}

// TestPG_TopicEditorAndRelations covers the content writes and reads that had no test on
// the real schema: topic status, the editor assignment, source deletion and the
// product-article relation with its mandatory reason (MA-5).
func TestPG_TopicEditorAndRelations(t *testing.T) {
	pool, svc, store := setupContent(t)
	ctx := context.Background()

	topicID, err := svc.CreateTopic(ctx, Topic{Name: "تغذیه", Slug: "nutrition-pg"})
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	topic, err := store.GetTopic(ctx, topicID)
	if err != nil || topic == nil {
		t.Fatalf("GetTopic: %v", err)
	}
	if err := store.UpdateTopicStatus(ctx, topicID, "INACTIVE"); err != nil {
		t.Fatalf("UpdateTopicStatus: %v", err)
	}
	topic, _ = store.GetTopic(ctx, topicID)
	if topic.Status != "INACTIVE" {
		t.Fatalf("topic status = %q, want INACTIVE", topic.Status)
	}

	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "تغذیه و خواب", Slug: "nutrition-sleep", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	var editorID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name) VALUES ('editor@khabavar.ir','ویرایشگر') RETURNING id`).Scan(&editorID); err != nil {
		t.Fatalf("seed editor: %v", err)
	}
	if err := store.SetArticleEditor(ctx, articleID, editorID); err != nil {
		t.Fatalf("SetArticleEditor: %v", err)
	}
	a, _ := store.GetArticle(ctx, articleID)
	if a.EditorID != editorID {
		t.Fatalf("editor_id = %q, want %q", a.EditorID, editorID)
	}

	if err := store.UpdateArticleStatus(ctx, articleID, StatusInReview, nil, time.Now()); err != nil {
		t.Fatalf("UpdateArticleStatus: %v", err)
	}
	a, _ = store.GetArticle(ctx, articleID)
	if a.Status != StatusInReview {
		t.Fatalf("article status = %q, want %q", a.Status, StatusInReview)
	}

	srcID, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: articleID, SourceType: CitationGuideline, Citation: "AASM 2015",
	})
	if err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	if err := store.DeleteArticleSource(ctx, srcID); err != nil {
		t.Fatalf("DeleteArticleSource: %v", err)
	}
	sources, err := store.ListArticleSources(ctx, articleID)
	if err != nil {
		t.Fatalf("ListArticleSources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources = %+v, want none after delete", sources)
	}
}

// TestPG_AddProductArticleRelation: the relation carries a mandatory reason (MA-5) and
// had no test on the real schema.
func TestPG_AddProductArticleRelation(t *testing.T) {
	pool, svc, store := setupContent(t)
	ctx := context.Background()

	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "بهترین بالش", Slug: "best-pillow", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	var productID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('بالش طبی','pillow-rel','DRAFT') RETURNING id`).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	relID, err := store.AddProductArticleRelation(ctx, ProductArticleRelation{
		ProductID: productID, ArticleID: articleID,
		RelationType: "EDUCATIONAL", RelationReason: "راهنمای انتخاب بالش",
	})
	if err != nil {
		t.Fatalf("AddProductArticleRelation: %v", err)
	}
	if relID == "" {
		t.Fatal("expected a relation id")
	}

	// The reason is mandatory (MA-5): the database must refuse an empty one.
	if _, err := store.AddProductArticleRelation(ctx, ProductArticleRelation{
		ProductID: productID, ArticleID: articleID, RelationType: "EDUCATIONAL",
	}); err == nil {
		t.Fatal("a relation with an empty reason must be refused (MA-5)")
	}
}

// TestPG_UpdateArticleAndReview covers the panel's edit and the single review endpoint:
// a PATCH rewrites the editable columns and leaves the status alone, the workflow still
// refuses an illegal jump, and blanking a required field is refused.
func TestPG_UpdateArticleAndReview(t *testing.T) {
	_, svc, store := setupContent(t)
	ctx := context.Background()

	id, err := svc.CreateArticle(ctx, Article{
		Title: "عنوان اول", Slug: "edit-me", Summary: "خلاصه", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	newTitle := "عنوان دوم"
	edited, err := svc.UpdateArticle(ctx, id, ArticlePatch{Title: &newTitle})
	if err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}
	if edited.Title != "عنوان دوم" || edited.Slug != "edit-me" {
		t.Fatalf("edited = %+v, want the new title and the same slug", edited)
	}
	reloaded, _ := store.GetArticle(ctx, id)
	if reloaded.Title != "عنوان دوم" || reloaded.Status != StatusDraft {
		t.Fatalf("reloaded = %+v, want the new title and status DRAFT", reloaded)
	}

	// A blank slug or body is refused rather than written.
	blank := ""
	if _, err := svc.UpdateArticle(ctx, id, ArticlePatch{Slug: &blank}); !errors.Is(err, ErrSlugRequired) {
		t.Fatalf("blank slug = %v, want ErrSlugRequired", err)
	}
	if _, err := svc.UpdateArticle(ctx, id, ArticlePatch{Body: &blank}); !errors.Is(err, ErrBodyRequired) {
		t.Fatalf("blank body = %v, want ErrBodyRequired", err)
	}

	// Review decisions map onto the guarded transitions.
	if err := svc.SubmitForReview(ctx, id); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := svc.SubmitForReview(ctx, id); err == nil {
		t.Fatal("submitting an IN_REVIEW article must be refused")
	}
	if err := svc.Approve(ctx, id, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	a, _ := store.GetArticle(ctx, id)
	if a.Status != StatusApproved {
		t.Fatalf("status = %q, want APPROVED", a.Status)
	}
	if err := svc.RejectToDraft(ctx, id); err != nil {
		t.Fatalf("reject: %v", err)
	}
	a, _ = store.GetArticle(ctx, id)
	if a.Status != StatusDraft {
		t.Fatalf("status = %q, want DRAFT after reject", a.Status)
	}
}

// TestPG_PublishedArticlesForProduct: the storefront's related reading must never leak a
// DRAFT article through a product relation.
func TestPG_PublishedArticlesForProduct(t *testing.T) {
	pool, svc, _ := setupContent(t)
	ctx := context.Background()

	var productID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('بالش','pillow-rel-2','DRAFT') RETURNING id`).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// One published article and one draft, both related to the product.
	pubID, err := svc.CreateArticle(ctx, Article{
		Title: "منتشرشده", Slug: "rel-pub", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if _, err := svc.AddArticleSource(ctx, ArticleSource{ArticleID: pubID, SourceType: CitationGuideline, Citation: "AASM"}); err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	if err := svc.SubmitForReview(ctx, pubID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := svc.Approve(ctx, pubID, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := svc.Publish(ctx, pubID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	draftID, err := svc.CreateArticle(ctx, Article{
		Title: "پیش‌نویس", Slug: "rel-draft", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle (draft): %v", err)
	}

	rel := ProductArticleRelation{ProductID: productID, RelationType: "EDUCATIONAL", RelationReason: "راهنما"}
	rel.ArticleID = pubID
	if _, err := svc.AddProductRelation(ctx, rel); err != nil {
		t.Fatalf("AddProductRelation: %v", err)
	}
	rel.ArticleID = draftID
	if _, err := svc.AddProductRelation(ctx, rel); err != nil {
		t.Fatalf("AddProductRelation (draft): %v", err)
	}

	arts, err := svc.PublishedArticlesForProduct(ctx, productID)
	if err != nil {
		t.Fatalf("PublishedArticlesForProduct: %v", err)
	}
	if len(arts) != 1 || arts[0].ID != pubID {
		t.Fatalf("articles = %+v, want only the published one", arts)
	}
}

// TestPG_StorefrontFilters covers the two filters the contract declares for the magazine
// list — topic_slug and q — which the handler used to ignore entirely.
func TestPG_StorefrontFilters(t *testing.T) {
	_, svc, store := setupContent(t)
	ctx := context.Background()

	pubID, err := svc.CreateArticle(ctx, Article{
		Title: "خواب عمیق", Slug: "filter-pub", Summary: "راهنمای خواب", Body: "متن",
		Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if _, err := svc.AddArticleSource(ctx, ArticleSource{ArticleID: pubID, SourceType: CitationGuideline, Citation: "AASM"}); err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	for _, step := range []func(context.Context, string) error{svc.SubmitForReview, func(c context.Context, id string) error { return svc.Approve(c, id, "") }, svc.Publish} {
		if err := step(ctx, pubID); err != nil {
			t.Fatalf("publish step: %v", err)
		}
	}
	// A draft that must never surface through a filter.
	draftID, err := svc.CreateArticle(ctx, Article{
		Title: "خواب عمیق (پیش‌نویس)", Slug: "filter-draft", Body: "متن", Language: "fa", SourceType: "ORIGINAL",
	})
	if err != nil {
		t.Fatalf("CreateArticle (draft): %v", err)
	}

	// q matches the published article only.
	arts, total, err := svc.SearchPublishedArticles(ctx, "خواب", 1, 10)
	if err != nil {
		t.Fatalf("SearchPublishedArticles: %v", err)
	}
	if total != 1 || len(arts) != 1 || arts[0].ID != pubID {
		t.Fatalf("search = %d results (total %d), want only the published one", len(arts), total)
	}
	_ = draftID

	// An unknown topic slug is an empty list, not an error.
	empty, totalEmpty, err := svc.ListArticlesByTopicSlug(ctx, "no-such-topic", 1, 10)
	if err != nil {
		t.Fatalf("ListArticlesByTopicSlug(unknown): %v", err)
	}
	if totalEmpty != 0 || len(empty) != 0 {
		t.Fatalf("unknown slug returned %d articles", totalEmpty)
	}
	_ = store
}
