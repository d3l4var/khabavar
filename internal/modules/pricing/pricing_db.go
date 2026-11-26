// pricing_db.go provides the pgx implementation of pricing.Store.
package pricing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed pricing.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateCostRecord(ctx context.Context, c CostRecord) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_cost_records
		   (variant_id, supply_type, supplier_id, purchase_cost, inbound_freight,
		    inspection_cost, packaging_cost, fulfillment_cost, estimated_payment_fee,
		    estimated_shipping_subsidy, expected_return_cost, currency, valid_from,
		    valid_until, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		c.VariantID, c.SupplyType, nullableStr(c.SupplierID), c.PurchaseCost,
		c.InboundFreight, c.InspectionCost, c.PackagingCost, c.FulfillmentCost,
		c.EstimatedPaymentFee, c.EstimatedShippingSubsidy, c.ExpectedReturnCost,
		c.Currency, c.ValidFrom, nullableTimePtr(c.ValidUntil), nullableStr(c.CreatedBy),
	).Scan(&id)
	return id, err
}

func (s *DBStore) ListCostRecordsByVariant(ctx context.Context, variantID string) ([]CostRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, variant_id, supply_type, supplier_id, purchase_cost, inbound_freight,
		        inspection_cost, packaging_cost, fulfillment_cost, estimated_payment_fee,
		        estimated_shipping_subsidy, expected_return_cost, currency, valid_from,
		        valid_until, created_by
		   FROM product_cost_records WHERE variant_id = $1 ORDER BY valid_from DESC`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCostRows(rows)
}

func (s *DBStore) GetValidCostRecord(ctx context.Context, variantID string, now time.Time) (*CostRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, variant_id, supply_type, supplier_id, purchase_cost, inbound_freight,
		        inspection_cost, packaging_cost, fulfillment_cost, estimated_payment_fee,
		        estimated_shipping_subsidy, expected_return_cost, currency, valid_from,
		        valid_until, created_by
		   FROM product_cost_records
		  WHERE variant_id = $1 AND valid_from <= $2
		    AND (valid_until IS NULL OR valid_until > $2)
		  ORDER BY valid_from DESC LIMIT 1`, variantID, now)
	c, err := scanCostRow(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *DBStore) CreatePriceRecord(ctx context.Context, p PriceRecord) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_price_records
		   (variant_id, selling_price, discount_policy, target_contribution_rate,
		    calculated_contribution, currency, valid_from, valid_until, approved_by)
		 VALUES ($1,$2,$3,$4::numeric,$5,$6,$7,$8,$9) RETURNING id`,
		p.VariantID, p.SellingPrice, nullableJSON(p.DiscountPolicy),
		nullableFloatPtr(p.TargetContributionRate), nullableInt64Ptr(p.CalculatedContribution),
		p.Currency, p.ValidFrom, nullableTimePtr(p.ValidUntil), nullableStr(p.ApprovedBy),
	).Scan(&id)
	return id, err
}

func (s *DBStore) ApprovePriceRecord(ctx context.Context, id, approverID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE product_price_records SET approved_by = $2 WHERE id = $1 AND approved_by IS NULL`,
		id, approverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	// Distinguish not-found vs already-approved.
	var approved pgtype.Text
	err = s.pool.QueryRow(ctx,
		`SELECT approved_by FROM product_price_records WHERE id = $1`, id).Scan(&approved)
	if err == pgx.ErrNoRows {
		return ErrVariantNotFound
	}
	if err != nil {
		return err
	}
	if approved.Valid {
		return ErrPriceAlreadyApproved
	}
	return nil
}

func (s *DBStore) GetValidPriceRecord(ctx context.Context, variantID string, now time.Time) (*PriceRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, variant_id, selling_price, discount_policy, target_contribution_rate::float8,
		        calculated_contribution, currency, valid_from, valid_until, approved_by
		   FROM product_price_records
		  WHERE variant_id = $1 AND approved_by IS NOT NULL
		    AND valid_from <= $2 AND (valid_until IS NULL OR valid_until > $2)
		  ORDER BY valid_from DESC LIMIT 1`, variantID, now)
	p, err := scanPriceRow(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *DBStore) ListPriceRecordsByVariant(ctx context.Context, variantID string) ([]PriceRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, variant_id, selling_price, discount_policy, target_contribution_rate::float8,
		        calculated_contribution, currency, valid_from, valid_until, approved_by
		   FROM product_price_records WHERE variant_id = $1 ORDER BY valid_from DESC`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPriceRows(rows)
}

// ── scan helpers ───────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCostRow(row rowScanner) (*CostRecord, error) {
	var c CostRecord
	var supplierID, createdBy pgtype.Text
	var validUntil pgtype.Timestamptz
	if err := row.Scan(&c.ID, &c.VariantID, &c.SupplyType, &supplierID, &c.PurchaseCost,
		&c.InboundFreight, &c.InspectionCost, &c.PackagingCost, &c.FulfillmentCost,
		&c.EstimatedPaymentFee, &c.EstimatedShippingSubsidy, &c.ExpectedReturnCost,
		&c.Currency, &c.ValidFrom, &validUntil, &createdBy); err != nil {
		return nil, err
	}
	if supplierID.Valid {
		c.SupplierID = supplierID.String
	}
	if createdBy.Valid {
		c.CreatedBy = createdBy.String
	}
	if validUntil.Valid {
		t := validUntil.Time
		c.ValidUntil = &t
	}
	return &c, nil
}

func scanCostRows(rows pgx.Rows) ([]CostRecord, error) {
	out := make([]CostRecord, 0)
	for rows.Next() {
		c, err := scanCostRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanPriceRow(row rowScanner) (*PriceRecord, error) {
	var p PriceRecord
	var discount []byte
	var targetRate pgtype.Float8
	var calcContrib pgtype.Int8
	var validUntil pgtype.Timestamptz
	var approvedBy pgtype.Text
	if err := row.Scan(&p.ID, &p.VariantID, &p.SellingPrice, &discount, &targetRate,
		&calcContrib, &p.Currency, &p.ValidFrom, &validUntil, &approvedBy); err != nil {
		return nil, err
	}
	if discount != nil {
		p.DiscountPolicy = append(p.DiscountPolicy, discount...)
	}
	if targetRate.Valid {
		v := targetRate.Float64
		p.TargetContributionRate = &v
	}
	if calcContrib.Valid {
		v := calcContrib.Int64
		p.CalculatedContribution = &v
	}
	if validUntil.Valid {
		t := validUntil.Time
		p.ValidUntil = &t
	}
	if approvedBy.Valid {
		p.ApprovedBy = approvedBy.String
	}
	return &p, nil
}

func scanPriceRows(rows pgx.Rows) ([]PriceRecord, error) {
	out := make([]PriceRecord, 0)
	for rows.Next() {
		p, err := scanPriceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// nullable helpers (kept local; pricing does not import catalog).
func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableTimePtr(t *time.Time) any {
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

func nullableFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableInt64Ptr(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)
