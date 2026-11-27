// search_db.go provides the pgx implementation of catalog.SearchStore.
package catalog

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// SearchProducts runs a full-text + filter search against the products table.
// The query is matched with ILIKE against title, short_description, description,
// and manufacturer. Category filter joins product_categories. Sleep-relevance
// and status are direct column filters.
func (s *DBStore) SearchProducts(ctx context.Context, f SearchFilter) ([]SearchResult, int, error) {
	var (
		clauses []string
		args    []any
	)
	idx := 1
	addArg := func(v any) string {
		args = append(args, v)
		idx++
		return intToStr(idx - 1)
	}
	join := ""
	if f.CategoryID != "" {
		join = " JOIN product_categories pc ON pc.product_id = p.id "
		clauses = append(clauses, "pc.category_id = $"+addArg(f.CategoryID))
	}
	// The contract filters by slug, not id: resolve it in SQL so the storefront can link
	// with the slug it already has and no extra round-trip is needed.
	if f.CategorySlug != "" {
		if join == "" {
			join = " JOIN product_categories pc ON pc.product_id = p.id "
		}
		join += " JOIN categories c ON c.id = pc.category_id "
		clauses = append(clauses, "c.slug = $"+addArg(f.CategorySlug))
	}
	if f.BrandSlug != "" {
		join += " JOIN brands b ON b.id = p.brand_id "
		clauses = append(clauses, "b.slug = $"+addArg(f.BrandSlug))
	}
	if f.Query != "" {
		q := "%" + strings.ToLower(f.Query) + "%"
		clauses = append(clauses, "(LOWER(p.title) LIKE $"+addArg(q)+
			" OR LOWER(p.short_description) LIKE $"+addArg(q)+
			" OR LOWER(p.description) LIKE $"+addArg(q)+
			" OR LOWER(p.manufacturer) LIKE $"+addArg(q)+")")
	}
	if f.SleepRelevance != "" {
		clauses = append(clauses, "p.sleep_relevance = $"+addArg(f.SleepRelevance))
	}
	if f.Status != "" {
		clauses = append(clauses, "p.status = $"+addArg(f.Status))
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	// Count total.
	countQuery := "SELECT count(*) FROM products p" + join + where
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch page. A price sort needs the current approved price per product, so it joins a
	// lateral subquery; the other sorts stay on the products row.
	priceJoin := ""
	switch f.Sort {
	case "price_asc", "price_desc":
		priceJoin = " LEFT JOIN LATERAL (" +
			"SELECT MIN(ppr.selling_price) AS price FROM product_price_records ppr " +
			"JOIN product_variants pv ON pv.id = ppr.variant_id " +
			"WHERE pv.product_id = p.id AND ppr.approved_by IS NOT NULL " +
			"AND ppr.valid_from <= now() AND (ppr.valid_until IS NULL OR ppr.valid_until > now())" +
			") pr ON true "
	}

	offset := (f.Page - 1) * f.PageSize
	listQuery := "SELECT p.id, p.title, p.slug, p.brand_id, p.short_description, p.description, " +
		"p.manufacturer, p.country_of_origin, p.sleep_relevance, p.spec_source, " +
		"p.spec_last_reviewed_at, p.status, p.published_at " +
		"FROM products p" + join + priceJoin + where +
		" " + orderBy(f.Sort) + " LIMIT $" + addArg(f.PageSize) +
		" OFFSET $" + addArg(offset)
	rows, err := s.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]SearchResult, 0)
	for rows.Next() {
		var p Product
		var brandID, shortDesc, desc, mfr, country, sleepRel, specSrc pgtype.Text
		var specReviewed, publishedAt pgtype.Timestamptz
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &brandID, &shortDesc, &desc,
			&mfr, &country, &sleepRel, &specSrc, &specReviewed, &p.Status,
			&publishedAt); err != nil {
			return nil, 0, err
		}
		p.BrandID = textVal(brandID)
		p.ShortDescription = textVal(shortDesc)
		p.Description = textVal(desc)
		p.Manufacturer = textVal(mfr)
		p.CountryOfOrigin = textVal(country)
		p.SleepRelevance = textVal(sleepRel)
		p.SpecSource = textVal(specSrc)
		if specReviewed.Valid {
			t := specReviewed.Time
			p.SpecLastReviewedAt = &t
		}
		if publishedAt.Valid {
			t := publishedAt.Time
			p.PublishedAt = &t
		}
		out = append(out, SearchResult{Product: p, Snippet: p.ShortDescription})
	}
	return out, total, rows.Err()
}

func intToStr(n int) string {
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

func textVal(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// Compile-time assertion that DBStore satisfies SearchStore.
var _ SearchStore = (*DBStore)(nil)

// orderBy maps the contract's sort enum to SQL. An unknown value falls back to the default
// order rather than erroring: the enum is advisory for a read endpoint.
func orderBy(sort string) string {
	switch sort {
	case "newest":
		return "ORDER BY p.published_at DESC NULLS LAST, p.created_at DESC"
	case "price_asc":
		return "ORDER BY pr.price ASC NULLS LAST, p.published_at DESC NULLS LAST"
	case "price_desc":
		return "ORDER BY pr.price DESC NULLS LAST, p.published_at DESC NULLS LAST"
	default: // relevance
		return "ORDER BY p.published_at DESC NULLS LAST"
	}
}
