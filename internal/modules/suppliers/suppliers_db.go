// suppliers_db.go provides the pgx implementation of suppliers.Store.
// The UpdateSupplierVariantPrice method is the critical one: it writes the new
// price on supplier_variants AND inserts a price_history row with valid_from=now,
// valid_until=NULL, while closing the previously-open history row's valid_until —
// all in one transaction (§7.3: every price change must be recorded in history).
package suppliers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed suppliers.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateSupplier(ctx context.Context, sup Supplier) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO suppliers (name, supplier_type, registration_number, contact_name,
		         phone, email, address, website, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		sup.Name, sup.SupplierType, nullableStr(sup.RegistrationNumber),
		nullableStr(sup.ContactName), nullableStr(sup.Phone), nullableStr(sup.Email),
		nullableStr(sup.Address), nullableStr(sup.Website), sup.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetSupplier(ctx context.Context, id string) (*Supplier, error) {
	var sup Supplier
	var sType, regNo, contact, phone, email, addr, website pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, supplier_type, registration_number, contact_name,
		         phone, email, address, website, status, created_at, updated_at
		   FROM suppliers WHERE id = $1`, id).
		Scan(&sup.ID, &sup.Name, &sType, &regNo, &contact, &phone, &email, &addr,
			&website, &sup.Status, &sup.CreatedAt, &sup.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sup.SupplierType = textOr(sType)
	sup.RegistrationNumber = textOr(regNo)
	sup.ContactName = textOr(contact)
	sup.Phone = textOr(phone)
	sup.Email = textOr(email)
	sup.Address = textOr(addr)
	sup.Website = textOr(website)
	return &sup, nil
}

func (s *DBStore) UpdateSupplierStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE suppliers SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrSupplierNotFound
	}
	return nil
}

func (s *DBStore) CreateSupplierProduct(ctx context.Context, sp SupplierProduct) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO supplier_products (supplier_id, product_id, supplier_product_code,
		         minimum_order_quantity, lead_time_days, status)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sp.SupplierID, sp.ProductID, nullableStr(sp.SupplierProductCode),
		nullableIntPtr(sp.MinimumOrderQty), nullableIntPtr(sp.LeadTimeDays), sp.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetSupplierProduct(ctx context.Context, id string) (*SupplierProduct, error) {
	var sp SupplierProduct
	var code pgtype.Text
	var moq, lead pgtype.Int4
	err := s.pool.QueryRow(ctx,
		`SELECT id, supplier_id, product_id, supplier_product_code,
		         minimum_order_quantity, lead_time_days, status
		   FROM supplier_products WHERE id = $1`, id).
		Scan(&sp.ID, &sp.SupplierID, &sp.ProductID, &code, &moq, &lead, &sp.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sp.SupplierProductCode = textOr(code)
	sp.MinimumOrderQty = intPtr(moq)
	sp.LeadTimeDays = intPtr(lead)
	return &sp, nil
}

func (s *DBStore) GetSupplierProductByPair(ctx context.Context, supplierID, productID string) (*SupplierProduct, error) {
	var sp SupplierProduct
	var code pgtype.Text
	var moq, lead pgtype.Int4
	err := s.pool.QueryRow(ctx,
		`SELECT id, supplier_id, product_id, supplier_product_code,
		         minimum_order_quantity, lead_time_days, status
		   FROM supplier_products WHERE supplier_id = $1 AND product_id = $2`,
		supplierID, productID).
		Scan(&sp.ID, &sp.SupplierID, &sp.ProductID, &code, &moq, &lead, &sp.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sp.SupplierProductCode = textOr(code)
	sp.MinimumOrderQty = intPtr(moq)
	sp.LeadTimeDays = intPtr(lead)
	return &sp, nil
}

// CreateSupplierVariant inserts the offer and, when a purchase price is given, its
// opening price_history row in the same transaction. Without that row
// ListPriceHistory would be an incomplete timeline: the first price would live only on
// supplier_variants and could never be audited (§7.3 "every price must be traced").
func (s *DBStore) CreateSupplierVariant(ctx context.Context, sv SupplierVariant) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO supplier_variants (supplier_product_id, variant_id, supplier_sku,
		         purchase_price, currency, available_quantity, last_stock_check_at,
		         price_valid_until, last_price_check_at, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		sv.SupplierProductID, sv.VariantID, nullableStr(sv.SupplierSKU),
		nullableI64Ptr(sv.PurchasePrice), sv.Currency, sv.AvailableQuantity,
		nullableTime(sv.LastStockCheckAt), nullableTime(sv.PriceValidUntil),
		nullableTime(sv.LastPriceCheckAt), sv.Status).Scan(&id); err != nil {
		return "", err
	}
	if sv.PurchasePrice != nil {
		if _, err := tx.Exec(ctx,
			`INSERT INTO supplier_variant_price_history
			   (supplier_variant_id, purchase_price, currency, valid_from, valid_until)
			 VALUES ($1,$2,$3,now(),NULL)`,
			id, *sv.PurchasePrice, sv.Currency); err != nil {
			return "", err
		}
	}
	return id, tx.Commit(ctx)
}

func (s *DBStore) GetSupplierVariant(ctx context.Context, id string) (*SupplierVariant, error) {
	var sv SupplierVariant
	var sku pgtype.Text
	var price pgtype.Int8
	var stockCheck, priceUntil, lastCheck pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, supplier_product_id, variant_id, supplier_sku, purchase_price,
		         currency, available_quantity, last_stock_check_at, price_valid_until,
		         last_price_check_at, status
		   FROM supplier_variants WHERE id = $1`, id).
		Scan(&sv.ID, &sv.SupplierProductID, &sv.VariantID, &sku, &price, &sv.Currency,
			&sv.AvailableQuantity, &stockCheck, &priceUntil, &lastCheck, &sv.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sv.SupplierSKU = textOr(sku)
	sv.PurchasePrice = i64Ptr(price)
	sv.LastStockCheckAt = timePtr(stockCheck)
	sv.PriceValidUntil = timePtr(priceUntil)
	sv.LastPriceCheckAt = timePtr(lastCheck)
	return &sv, nil
}

func (s *DBStore) ListSupplierVariantsByVariant(ctx context.Context, variantID string) ([]SupplierVariant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, supplier_product_id, variant_id, supplier_sku, purchase_price,
		         currency, available_quantity, last_stock_check_at, price_valid_until,
		         last_price_check_at, status
		   FROM supplier_variants WHERE variant_id = $1 ORDER BY created_at`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SupplierVariant, 0)
	for rows.Next() {
		var sv SupplierVariant
		var sku pgtype.Text
		var price pgtype.Int8
		var stockCheck, priceUntil, lastCheck pgtype.Timestamptz
		if err := rows.Scan(&sv.ID, &sv.SupplierProductID, &sv.VariantID, &sku, &price,
			&sv.Currency, &sv.AvailableQuantity, &stockCheck, &priceUntil, &lastCheck, &sv.Status); err != nil {
			return nil, err
		}
		sv.SupplierSKU = textOr(sku)
		sv.PurchasePrice = i64Ptr(price)
		sv.LastStockCheckAt = timePtr(stockCheck)
		sv.PriceValidUntil = timePtr(priceUntil)
		sv.LastPriceCheckAt = timePtr(lastCheck)
		out = append(out, sv)
	}
	return out, rows.Err()
}

// UpdateSupplierVariantPrice is the §7.3 transactional price change. It:
//  1. Opens a transaction.
//  2. Closes the previously-open history row (valid_until = now).
//  3. Updates supplier_variants.purchase_price + price_valid_until.
//  4. Inserts a new price_history row (valid_from=now, valid_until=NULL).
//  5. Commits. On any error the transaction rolls back — the price and history
//     stay consistent (no orphan open history rows, no price change without history).
func (s *DBStore) UpdateSupplierVariantPrice(ctx context.Context, id string, newPrice int64, validFrom time.Time, validUntil *time.Time, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Close the previous open history row.
	if _, err := tx.Exec(ctx,
		`UPDATE supplier_variant_price_history
		    SET valid_until = $2
		  WHERE supplier_variant_id = $1 AND valid_until IS NULL`,
		id, now); err != nil {
		return err
	}
	// 2. Update the supplier_variants row.
	ct, err := tx.Exec(ctx,
		`UPDATE supplier_variants
		    SET purchase_price = $2, price_valid_until = $3, last_price_check_at = $4
		  WHERE id = $1`,
		id, newPrice, nullableTime(validUntil), now)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrSupplierVariantNotFound
	}
	// 3. Insert the new history row.
	if _, err := tx.Exec(ctx,
		`INSERT INTO supplier_variant_price_history
		   (supplier_variant_id, purchase_price, currency, valid_from, valid_until)
		 VALUES ($1,$2,'IRR',$3,NULL)`,
		id, newPrice, validFrom); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *DBStore) ListPriceHistory(ctx context.Context, supplierVariantID string) ([]PriceHistoryEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, supplier_variant_id, purchase_price, currency, valid_from, valid_until
		   FROM supplier_variant_price_history
		  WHERE supplier_variant_id = $1
		  ORDER BY valid_from DESC`, supplierVariantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PriceHistoryEntry, 0)
	for rows.Next() {
		var e PriceHistoryEntry
		var validUntil pgtype.Timestamptz
		if err := rows.Scan(&e.ID, &e.SupplierVariantID, &e.PurchasePrice, &e.Currency,
			&e.ValidFrom, &validUntil); err != nil {
			return nil, err
		}
		e.ValidUntil = timePtr(validUntil)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *DBStore) CreateEvaluation(ctx context.Context, ev Evaluation) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO supplier_evaluations
		   (supplier_id, quality_score, pricing_score, availability_score, warranty_score,
		    documentation_score, flexibility_score, logistics_score, communication_score,
		    total_score, evaluation_date, reviewer_id, weights_version, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		ev.SupplierID, ev.QualityScore, ev.PricingScore, ev.AvailabilityScore,
		ev.WarrantyScore, ev.DocumentationScore, ev.FlexibilityScore,
		ev.LogisticsScore, ev.CommunicationScore, ev.TotalScore, ev.EvaluationDate,
		nullableStr(ev.ReviewerID), ev.WeightsVersion, nullableStr(ev.Notes)).Scan(&id)
	return id, err
}

func (s *DBStore) ListEvaluationsBySupplier(ctx context.Context, supplierID string) ([]Evaluation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, supplier_id, quality_score, pricing_score, availability_score,
		         warranty_score, documentation_score, flexibility_score, logistics_score,
		         communication_score, total_score, evaluation_date, reviewer_id,
		         weights_version, notes
		   FROM supplier_evaluations
		  WHERE supplier_id = $1
		  ORDER BY evaluation_date DESC`, supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Evaluation, 0)
	for rows.Next() {
		var ev Evaluation
		var reviewer, notes pgtype.Text
		if err := rows.Scan(&ev.ID, &ev.SupplierID, &ev.QualityScore, &ev.PricingScore,
			&ev.AvailabilityScore, &ev.WarrantyScore, &ev.DocumentationScore,
			&ev.FlexibilityScore, &ev.LogisticsScore, &ev.CommunicationScore,
			&ev.TotalScore, &ev.EvaluationDate, &reviewer, &ev.WeightsVersion, &notes); err != nil {
			return nil, err
		}
		ev.ReviewerID = textOr(reviewer)
		ev.Notes = textOr(notes)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ── scan/nullable helpers ──────────────────────────────────────────────────────

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

func i64Ptr(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := int64(i.Int64)
	return &v
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

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullableIntPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableI64Ptr(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)
