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

	// Fetch page.
	offset := (f.Page - 1) * f.PageSize
	listQuery := "SELECT p.id, p.title, p.slug, p.brand_id, p.short_description, p.description, " +
		"p.manufacturer, p.country_of_origin, p.sleep_relevance, p.spec_source, " +
		"p.spec_last_reviewed_at, p.status, p.published_at " +
		"FROM products p" + join + where +
		" ORDER BY p.published_at DESC NULLS LAST LIMIT $" + addArg(f.PageSize) +
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
