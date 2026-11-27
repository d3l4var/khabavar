// catalog_db.go provides the pgx implementation of catalog.Store.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBCategoryStore/DBStore is the pgx-backed catalog.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateCategory(ctx context.Context, c Category) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO categories (parent_id, name, slug, description, image_id, sort_order, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		nullableStr(c.ParentID), c.Name, c.Slug, nullableStr(c.Description),
		nullableStr(c.ImageID), c.SortOrder, c.Status).Scan(&id)
	return id, err
}

func (s *DBStore) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_id, name, slug, description, image_id, sort_order, status
		   FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Category, 0)
	for rows.Next() {
		var c Category
		var parentID, desc, imageID pgtype.Text
		if err := rows.Scan(&c.ID, &parentID, &c.Name, &c.Slug, &desc, &imageID, &c.SortOrder, &c.Status); err != nil {
			return nil, err
		}
		if parentID.Valid {
			c.ParentID = parentID.String
		}
		if desc.Valid {
			c.Description = desc.String
		}
		if imageID.Valid {
			c.ImageID = imageID.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *DBStore) CreateProduct(ctx context.Context, p Product) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO products
		   (title, slug, brand_id, short_description, description, manufacturer,
		    country_of_origin, sleep_relevance, spec_source, spec_last_reviewed_at, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		p.Title, p.Slug, nullableStr(p.BrandID), nullableStr(p.ShortDescription),
		nullableStr(p.Description), nullableStr(p.Manufacturer), nullableStr(p.CountryOfOrigin),
		nullableStr(p.SleepRelevance), nullableStr(p.SpecSource),
		nullableTime(p.SpecLastReviewedAt), p.Status).Scan(&id)
	if err != nil {
		// A duplicate slug is the operator's mistake, not a server fault: two products cannot
		// share a URL. It used to surface as 500 "server error", which told nobody anything.
		if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", ErrSlugTaken
		}
		return "", err
	}
	for _, cid := range p.CategoryIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO product_categories (product_id, category_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, id, cid); err != nil {
			return "", fmt.Errorf("link category %s: %w", cid, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *DBStore) GetProduct(ctx context.Context, id string) (*Product, error) {
	var p Product
	var brandID, shortDesc, desc, manufacturer, origin, sleepRel, specSrc pgtype.Text
	var specReviewed, publishedAt pgtype.Timestamptz
	// The storefront links products by SLUG; the panel uses the uuid. id is a uuid column, so
	// passing a slug straight through was a driver error and the product page answered 500.
	where := `WHERE id = $1`
	if _, err := uuid.Parse(id); err != nil {
		where = `WHERE slug = $1`
	}
	err := s.pool.QueryRow(ctx,
		`SELECT id, title, slug, brand_id, short_description, description, manufacturer,
		        country_of_origin, sleep_relevance, spec_source, spec_last_reviewed_at,
		        status, published_at
		   FROM products `+where, id).
		Scan(&p.ID, &p.Title, &p.Slug, &brandID, &shortDesc, &desc, &manufacturer,
			&origin, &sleepRel, &specSrc, &specReviewed, &p.Status, &publishedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.BrandID = textOr(brandID)
	p.ShortDescription = textOr(shortDesc)
	p.Description = textOr(desc)
	p.Manufacturer = textOr(manufacturer)
	p.CountryOfOrigin = textOr(origin)
	p.SleepRelevance = textOr(sleepRel)
	p.SpecSource = textOr(specSrc)
	if specReviewed.Valid {
		t := specReviewed.Time
		p.SpecLastReviewedAt = &t
	}
	if publishedAt.Valid {
		t := publishedAt.Time
		p.PublishedAt = &t
	}
	// Load category ids. Use the resolved p.ID: the caller may have passed a slug.
	rows, err := s.pool.Query(ctx,
		`SELECT category_id FROM product_categories WHERE product_id = $1`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		p.CategoryIDs = append(p.CategoryIDs, cid)
	}
	return &p, rows.Err()
}

func (s *DBStore) ListProductsByCategory(ctx context.Context, categoryID, status string, page, pageSize int) ([]Product, int, error) {
	offset := (page - 1) * pageSize
	// Count first (status filter optional).
	var total int
	countQ := `SELECT count(*) FROM product_categories pc
	            JOIN products p ON p.id = pc.product_id
	           WHERE pc.category_id = $1`
	args := []any{categoryID}
	if status != "" {
		countQ += " AND p.status = $2"
		args = append(args, status)
	}
	if err := s.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 || offset >= total {
		return []Product{}, total, nil
	}
	listQ := `SELECT p.id, p.title, p.slug, p.brand_id, p.short_description, p.description,
	                 p.manufacturer, p.country_of_origin, p.sleep_relevance, p.spec_source,
	                 p.spec_last_reviewed_at, p.status, p.published_at
	            FROM product_categories pc
	            JOIN products p ON p.id = pc.product_id
	           WHERE pc.category_id = $1`
	largs := []any{categoryID, pageSize, offset}
	if status != "" {
		listQ += " AND p.status = $2 ORDER BY p.created_at DESC LIMIT $3 OFFSET $4"
		// shift: status is $2, pageSize $3, offset $4
		largs = []any{categoryID, status, pageSize, offset}
	} else {
		listQ += " ORDER BY p.created_at DESC LIMIT $2 OFFSET $3"
	}
	rows, err := s.pool.Query(ctx, listQ, largs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanProducts(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// UpdateProductStatus changes a product's status. A zero-row update means the product
// does not exist, which must not be reported as success.
func (s *DBStore) UpdateProductStatus(ctx context.Context, id, status string, publishedAt *time.Time) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE products SET status = $2, published_at = COALESCE($3, published_at) WHERE id = $1`,
		id, status, nullableTime(publishedAt))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	return nil
}

func (s *DBStore) CreateVariant(ctx context.Context, v Variant) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_variants
		   (product_id, sku, barcode, attributes, weight_grams, length_mm, width_mm, height_mm, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		v.ProductID, v.SKU, nullableStr(v.Barcode), nullableJSON(v.Attributes),
		nullableIntPtr(v.WeightGrams), nullableIntPtr(v.LengthMm),
		nullableIntPtr(v.WidthMm), nullableIntPtr(v.HeightMm), v.Status).Scan(&id)
	return id, err
}

func (s *DBStore) ListVariantsByProduct(ctx context.Context, productID string) ([]Variant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, product_id, sku, barcode, attributes, weight_grams, length_mm, width_mm, height_mm, status
		   FROM product_variants WHERE product_id = $1 ORDER BY created_at`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Variant, 0)
	for rows.Next() {
		var v Variant
		var barcode pgtype.Text
		var attrs []byte
		var weight, length, width, height pgtype.Int4
		if err := rows.Scan(&v.ID, &v.ProductID, &v.SKU, &barcode, &attrs,
			&weight, &length, &width, &height, &v.Status); err != nil {
			return nil, err
		}
		if barcode.Valid {
			v.Barcode = barcode.String
		}
		if attrs != nil {
			v.Attributes = append(v.Attributes, attrs...)
		}
		v.WeightGrams = intPtr(weight)
		v.LengthMm = intPtr(length)
		v.WidthMm = intPtr(width)
		v.HeightMm = intPtr(height)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *DBStore) GetVariantBySKU(ctx context.Context, sku string) (*Variant, error) {
	var v Variant
	var barcode pgtype.Text
	var attrs []byte
	var weight, length, width, height pgtype.Int4
	err := s.pool.QueryRow(ctx,
		`SELECT id, product_id, sku, barcode, attributes, weight_grams, length_mm, width_mm, height_mm, status
		   FROM product_variants WHERE sku = $1`, sku).
		Scan(&v.ID, &v.ProductID, &v.SKU, &barcode, &attrs, &weight, &length, &width, &height, &v.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if barcode.Valid {
		v.Barcode = barcode.String
	}
	if attrs != nil {
		v.Attributes = append(v.Attributes, attrs...)
	}
	v.WeightGrams = intPtr(weight)
	v.LengthMm = intPtr(length)
	v.WidthMm = intPtr(width)
	v.HeightMm = intPtr(height)
	return &v, nil
}

// ── scan helpers ───────────────────────────────────────────────────────────────

func scanProducts(rows pgx.Rows) ([]Product, error) {
	out := make([]Product, 0)
	for rows.Next() {
		var p Product
		var brandID, shortDesc, desc, manufacturer, origin, sleepRel, specSrc pgtype.Text
		var specReviewed, publishedAt pgtype.Timestamptz
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &brandID, &shortDesc, &desc, &manufacturer,
			&origin, &sleepRel, &specSrc, &specReviewed, &p.Status, &publishedAt); err != nil {
			return nil, err
		}
		p.BrandID = textOr(brandID)
		p.ShortDescription = textOr(shortDesc)
		p.Description = textOr(desc)
		p.Manufacturer = textOr(manufacturer)
		p.CountryOfOrigin = textOr(origin)
		p.SleepRelevance = textOr(sleepRel)
		p.SpecSource = textOr(specSrc)
		if specReviewed.Valid {
			t := specReviewed.Time
			p.SpecLastReviewedAt = &t
		}
		if publishedAt.Valid {
			t := publishedAt.Time
			p.PublishedAt = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func textOr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

func intPtr(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
	return &v
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullableIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)

// UpdateProduct writes the editable columns of a product and, when replaceCategories
// is true, replaces its category links in the same transaction (so a partial failure
// cannot leave the product attached to half its categories).
func (s *DBStore) UpdateProduct(ctx context.Context, p Product, replaceCategories bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx,
		`UPDATE products
		    SET title = $2, slug = $3, brand_id = NULLIF($4,'')::uuid,
		        short_description = $5, description = $6, manufacturer = $7,
		        country_of_origin = $8, sleep_relevance = NULLIF($9,''),
		        spec_source = NULLIF($10,''), spec_last_reviewed_at = $11,
		        updated_at = now()
		  WHERE id = $1`,
		p.ID, p.Title, p.Slug, p.BrandID, p.ShortDescription, p.Description,
		p.Manufacturer, p.CountryOfOrigin, p.SleepRelevance, p.SpecSource,
		p.SpecLastReviewedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	if replaceCategories {
		if _, err := tx.Exec(ctx, `DELETE FROM product_categories WHERE product_id = $1`, p.ID); err != nil {
			return err
		}
		for _, cid := range p.CategoryIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO product_categories (product_id, category_id) VALUES ($1,$2)
				 ON CONFLICT DO NOTHING`, p.ID, cid); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// Compile-time assertion that DBStore can edit products.
var _ ProductUpdateStore = (*DBStore)(nil)
