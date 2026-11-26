// content_db.go provides the pgx implementation of content.Store.
package content

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed content.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

// ── Topics ────────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTopic(ctx context.Context, t Topic) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO topics (name, slug, parent_id, sort_order, status)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		t.Name, t.Slug, nullableStr(t.ParentID), t.SortOrder, t.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetTopic(ctx context.Context, id string) (*Topic, error) {
	var t Topic
	var parentID pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, parent_id, sort_order, status, created_at
		   FROM topics WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.Slug, &parentID, &t.SortOrder, &t.Status, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.ParentID = textOr(parentID)
	return &t, nil
}

func (s *DBStore) ListTopics(ctx context.Context) ([]Topic, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, parent_id, sort_order, status, created_at
		   FROM topics WHERE status = 'ACTIVE' ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Topic, 0)
	for rows.Next() {
		var t Topic
		var parentID pgtype.Text
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &parentID, &t.SortOrder, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.ParentID = textOr(parentID)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *DBStore) UpdateTopicStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE topics SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrTopicNotFound
	}
	return nil
}

// ── Articles ──────────────────────────────────────────────────────────────────

func (s *DBStore) CreateArticle(ctx context.Context, a Article) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO articles (title, slug, summary, body, language, source_type,
		         source_url, parent_article_id, author_id, editor_id, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		a.Title, a.Slug, nullableStr(a.Summary), a.Body, a.Language, a.SourceType,
		nullableStr(a.SourceURL), nullableStr(a.ParentArticleID),
		nullableStr(a.AuthorID), nullableStr(a.EditorID), a.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetArticle(ctx context.Context, id string) (*Article, error) {
	return s.getArticleByQuery(ctx,
		`SELECT id, title, slug, summary, body, language, source_type, source_url,
		         parent_article_id, author_id, editor_id, status, published_at,
		         created_at, updated_at
		   FROM articles WHERE id = $1`, id)
}

func (s *DBStore) GetArticleBySlug(ctx context.Context, slug string) (*Article, error) {
	return s.getArticleByQuery(ctx,
		`SELECT id, title, slug, summary, body, language, source_type, source_url,
		         parent_article_id, author_id, editor_id, status, published_at,
		         created_at, updated_at
		   FROM articles WHERE slug = $1`, slug)
}

func (s *DBStore) getArticleByQuery(ctx context.Context, query string, arg any) (*Article, error) {
	var a Article
	var summary, sourceURL, parentID, authorID, editorID pgtype.Text
	var publishedAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, query, arg).
		Scan(&a.ID, &a.Title, &a.Slug, &summary, &a.Body, &a.Language, &a.SourceType,
			&sourceURL, &parentID, &authorID, &editorID, &a.Status, &publishedAt,
			&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Summary = textOr(summary)
	a.SourceURL = textOr(sourceURL)
	a.ParentArticleID = textOr(parentID)
	a.AuthorID = textOr(authorID)
	a.EditorID = textOr(editorID)
	a.PublishedAt = timePtr(publishedAt)
	return &a, nil
}

func (s *DBStore) UpdateArticleStatus(ctx context.Context, id, status string, publishedAt *time.Time, now time.Time) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE articles SET status = $2, published_at = COALESCE($3, published_at), updated_at = $4
		  WHERE id = $1`,
		id, status, publishedAt, now)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrArticleNotFound
	}
	return nil
}

func (s *DBStore) ListPublishedArticles(ctx context.Context, page, pageSize int) ([]Article, int, error) {
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM articles WHERE status = 'PUBLISHED'`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, slug, summary, body, language, source_type, source_url,
		         parent_article_id, author_id, editor_id, status, published_at,
		         created_at, updated_at
		   FROM articles WHERE status = 'PUBLISHED'
		  ORDER BY published_at DESC LIMIT $1 OFFSET $2`,
		pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanArticles(rows, total)
}

func (s *DBStore) ListArticlesByTopic(ctx context.Context, topicID string, page, pageSize int) ([]Article, int, error) {
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM articles a
		  JOIN article_topics at ON at.article_id = a.id
		  WHERE at.topic_id = $1 AND a.status = 'PUBLISHED'`, topicID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.title, a.slug, a.summary, a.body, a.language, a.source_type,
		         a.source_url, a.parent_article_id, a.author_id, a.editor_id, a.status,
		         a.published_at, a.created_at, a.updated_at
		   FROM articles a
		  JOIN article_topics at ON at.article_id = a.id
		  WHERE at.topic_id = $1 AND a.status = 'PUBLISHED'
		  ORDER BY a.published_at DESC LIMIT $2 OFFSET $3`,
		topicID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanArticles(rows, total)
}

func (s *DBStore) ListTranslations(ctx context.Context, parentArticleID string) ([]Article, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, slug, summary, body, language, source_type, source_url,
		         parent_article_id, author_id, editor_id, status, published_at,
		         created_at, updated_at
		   FROM articles WHERE parent_article_id = $1 ORDER BY language`, parentArticleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, _, _ := scanArticles(rows, 0)
	return out, nil
}

func (s *DBStore) SetArticleEditor(ctx context.Context, id, editorID string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE articles SET editor_id = $2, updated_at = now() WHERE id = $1`,
		id, nullableStr(editorID))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrArticleNotFound
	}
	return nil
}

// ── Article sources ───────────────────────────────────────────────────────────

func (s *DBStore) AddArticleSource(ctx context.Context, src ArticleSource) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO article_sources (article_id, source_type, citation, url)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		src.ArticleID, src.SourceType, src.Citation, nullableStr(src.URL)).Scan(&id)
	return id, err
}

func (s *DBStore) ListArticleSources(ctx context.Context, articleID string) ([]ArticleSource, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, article_id, source_type, citation, url, created_at
		   FROM article_sources WHERE article_id = $1 ORDER BY created_at`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ArticleSource, 0)
	for rows.Next() {
		var src ArticleSource
		var url pgtype.Text
		if err := rows.Scan(&src.ID, &src.ArticleID, &src.SourceType, &src.Citation, &url, &src.CreatedAt); err != nil {
			return nil, err
		}
		src.URL = textOr(url)
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *DBStore) DeleteArticleSource(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM article_sources WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrSourceNotFound
	}
	return nil
}

// ── Article-topic links ───────────────────────────────────────────────────────

func (s *DBStore) LinkArticleTopic(ctx context.Context, articleID, topicID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO article_topics (article_id, topic_id) VALUES ($1,$2)
		 ON CONFLICT (article_id, topic_id) DO NOTHING`, articleID, topicID)
	return err
}

func (s *DBStore) UnlinkArticleTopic(ctx context.Context, articleID, topicID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM article_topics WHERE article_id = $1 AND topic_id = $2`,
		articleID, topicID)
	return err
}

func (s *DBStore) ListTopicsForArticle(ctx context.Context, articleID string) ([]Topic, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.name, t.slug, t.parent_id, t.sort_order, t.status, t.created_at
		   FROM topics t JOIN article_topics at ON at.topic_id = t.id
		  WHERE at.article_id = $1 ORDER BY t.sort_order`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Topic, 0)
	for rows.Next() {
		var t Topic
		var parentID pgtype.Text
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &parentID, &t.SortOrder, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.ParentID = textOr(parentID)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── Product-article relations ─────────────────────────────────────────────────

func (s *DBStore) AddProductArticleRelation(ctx context.Context, rel ProductArticleRelation) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_article_relations (product_id, article_id, relation_type, relation_reason)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		rel.ProductID, rel.ArticleID, rel.RelationType, rel.RelationReason).Scan(&id)
	return id, err
}

func (s *DBStore) ListRelationsForArticle(ctx context.Context, articleID string) ([]ProductArticleRelation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, product_id, article_id, relation_type, relation_reason, created_at
		   FROM product_article_relations WHERE article_id = $1 ORDER BY created_at`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelations(rows)
}

func (s *DBStore) ListRelationsForProduct(ctx context.Context, productID string) ([]ProductArticleRelation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, product_id, article_id, relation_type, relation_reason, created_at
		   FROM product_article_relations WHERE product_id = $1 ORDER BY created_at`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRelations(rows)
}

func (s *DBStore) RemoveProductArticleRelation(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM product_article_relations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrRelationNotFound
	}
	return nil
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanArticles(rows pgx.Rows, total int) ([]Article, int, error) {
	out := make([]Article, 0)
	for rows.Next() {
		var a Article
		var summary, sourceURL, parentID, authorID, editorID pgtype.Text
		var publishedAt pgtype.Timestamptz
		if err := rows.Scan(&a.ID, &a.Title, &a.Slug, &summary, &a.Body, &a.Language,
			&a.SourceType, &sourceURL, &parentID, &authorID, &editorID, &a.Status,
			&publishedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		a.Summary = textOr(summary)
		a.SourceURL = textOr(sourceURL)
		a.ParentArticleID = textOr(parentID)
		a.AuthorID = textOr(authorID)
		a.EditorID = textOr(editorID)
		a.PublishedAt = timePtr(publishedAt)
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func scanRelations(rows pgx.Rows) ([]ProductArticleRelation, error) {
	out := make([]ProductArticleRelation, 0)
	for rows.Next() {
		var rel ProductArticleRelation
		if err := rows.Scan(&rel.ID, &rel.ProductID, &rel.ArticleID, &rel.RelationType,
			&rel.RelationReason, &rel.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ── nullable helpers ──────────────────────────────────────────────────────────

func textOr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)

// UpdateArticle rewrites the editable columns of an article. The status is not editable
// here: the workflow has its own guarded transitions (SAD §۷.۲).
func (s *DBStore) UpdateArticle(ctx context.Context, a Article) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE articles
		    SET title = $2, slug = $3, summary = $4, body = $5, language = $6, updated_at = now()
		  WHERE id = $1`,
		a.ID, a.Title, a.Slug, a.Summary, a.Body, a.Language)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrArticleNotFound
	}
	return nil
}

// ListPublishedArticlesForProduct returns the published articles linked to a product.
// The storefront must never see a DRAFT article through a product relation, so the
// status filter lives in SQL rather than in the caller.
func (s *DBStore) ListPublishedArticlesForProduct(ctx context.Context, productID string) ([]Article, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.title, a.slug, COALESCE(a.summary,''), '', COALESCE(a.language,'fa'),
		        COALESCE(a.source_type,''), COALESCE(a.source_url,''), COALESCE(a.parent_article_id::text,''),
		        COALESCE(a.author_id::text,''), COALESCE(a.editor_id::text,''), a.status,
		        a.published_at, a.created_at, a.updated_at
		   FROM product_article_relations r
		   JOIN articles a ON a.id = r.article_id
		  WHERE r.product_id = $1 AND a.status = 'PUBLISHED'
		  ORDER BY a.published_at DESC NULLS LAST`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Article, 0, 4)
	for rows.Next() {
		var a Article
		var published pgtype.Timestamptz
		if err := rows.Scan(&a.ID, &a.Title, &a.Slug, &a.Summary, &a.Body, &a.Language,
			&a.SourceType, &a.SourceURL, &a.ParentArticleID, &a.AuthorID, &a.EditorID,
			&a.Status, &published, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if published.Valid {
			t := published.Time
			a.PublishedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
