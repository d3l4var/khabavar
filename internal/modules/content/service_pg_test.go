package content

import (
	"context"
	"errors"
	"testing"
)

// TestPG_ContentServiceEdges covers the service branches the HTTP test does not reach:
// translations, sources, product relations, topic links and the guard clauses that return a
// sentinel instead of touching the database.
func TestPG_ContentServiceEdges(t *testing.T) {
	pool, svc, _ := setupContent(t)
	ctx := context.Background()
	_ = pool

	topicID, err := svc.CreateTopic(ctx, Topic{Name: "خواب", Slug: "sleep-edges"})
	if err != nil {
		t.Fatalf("topic: %v", err)
	}
	articleID, err := svc.CreateArticle(ctx, Article{
		Title: "راهنمای خواب", Slug: "sleep-guide", Body: "متن مقاله",
	})
	if err != nil {
		t.Fatalf("article: %v", err)
	}

	// --- guard clauses: an empty id is a sentinel, not a query ---
	if _, err := svc.GetArticle(ctx, ""); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("GetArticle(\"\") = %v", err)
	}
	if _, err := svc.ListTranslations(ctx, ""); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("ListTranslations(\"\") = %v", err)
	}
	if err := svc.DeleteArticleSource(ctx, ""); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("DeleteArticleSource(\"\") = %v", err)
	}
	if _, err := svc.ListRelationsForArticle(ctx, ""); !errors.Is(err, ErrArticleNotFound) {
		t.Fatalf("ListRelationsForArticle(\"\") = %v", err)
	}

	// --- translations (SAD §7.2): a translation needs a parent ---
	if _, err := svc.CreateArticle(ctx, Article{
		Title: "Sleep guide", Slug: "sleep-guide-en", Body: "body", SourceType: SourceTranslation,
	}); err == nil {
		t.Fatal("a translation without parent_article_id was accepted")
	}
	transID, err := svc.CreateArticle(ctx, Article{
		Title: "Sleep guide", Slug: "sleep-guide-en", Body: "body", SourceType: SourceTranslation,
		ParentArticleID: articleID, Language: "en",
	})
	if err != nil {
		t.Fatalf("translation: %v", err)
	}
	trans, err := svc.ListTranslations(ctx, articleID)
	if err != nil {
		t.Fatalf("list translations: %v", err)
	}
	if len(trans) != 1 || trans[0].ID != transID {
		t.Fatalf("translations = %+v", trans)
	}

	// An unknown source type is refused.
	if _, err := svc.CreateArticle(ctx, Article{
		Title: "x", Slug: "x-src", Body: "b", SourceType: "BOGUS",
	}); !errors.Is(err, ErrInvalidArticleSource) {
		t.Fatalf("bogus source type = %v", err)
	}

	// --- sources: add, list, delete, and delete again ---
	srcID, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: articleID, SourceType: "JOURNAL", Citation: "منبع", URL: "https://example.com/a",
	})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	sources, err := svc.ListArticleSources(ctx, articleID)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %+v, %v", sources, err)
	}
	if err := svc.DeleteArticleSource(ctx, srcID); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	if sources, _ := svc.ListArticleSources(ctx, articleID); len(sources) != 0 {
		t.Fatalf("source survived the delete: %+v", sources)
	}

	// --- product relations (MA-5): the reason is mandatory ---
	if _, err := svc.AddProductRelation(ctx, ProductArticleRelation{
		ArticleID: articleID, ProductID: "00000000-0000-0000-0000-000000000001",
		RelationType: RelationRelated,
	}); !errors.Is(err, ErrRelationReasonReq) {
		t.Fatalf("relation without a reason = %v", err)
	}
	if _, err := svc.AddProductRelation(ctx, ProductArticleRelation{
		ArticleID: articleID, ProductID: "00000000-0000-0000-0000-000000000001",
		RelationType: "BOGUS", RelationReason: "چون",
	}); !errors.Is(err, ErrInvalidRelationType) {
		t.Fatalf("bogus relation type = %v", err)
	}
	if _, err := svc.AddProductRelation(ctx, ProductArticleRelation{
		RelationType: RelationRelated, RelationReason: "چون",
	}); !errors.Is(err, ErrRelationNotFound) {
		t.Fatalf("relation without ids = %v", err)
	}

	// --- topic links ---
	if err := svc.LinkTopic(ctx, articleID, topicID); err != nil {
		t.Fatalf("link topic: %v", err)
	}
	topics, err := svc.ListTopicsForArticle(ctx, articleID)
	if err != nil {
		t.Fatalf("topics for article: %v", err)
	}
	if len(topics) == 0 {
		t.Fatal("the linked topic is not listed")
	}

	// --- the storefront reads ---
	if _, _, err := svc.ListPublishedArticles(ctx, 1, 20); err != nil {
		t.Fatalf("published list: %v", err)
	}
	if _, _, err := svc.ListArticlesByTopic(ctx, topicID, 1, 20); err != nil {
		t.Fatalf("by topic: %v", err)
	}
	// A draft article must not appear in the published list.
	published, _, _ := svc.ListPublishedArticles(ctx, 1, 100)
	for _, a := range published {
		if a.ID == articleID {
			t.Fatal("a DRAFT article appears in the published list")
		}
	}
}
