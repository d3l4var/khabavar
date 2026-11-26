package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CustomerProfile is the /me view (CIAM §۵): no internal columns, no marketing flags
// beyond what the customer themselves can change.
type CustomerProfile struct {
	ID            string `json:"id"`
	Phone         string `json:"phone,omitempty"`
	Email         string `json:"email,omitempty"`
	FullName      string `json:"full_name,omitempty"`
	Status        string `json:"status"`
	PhoneVerified bool   `json:"phone_verified"`
}

// Address is one saved shipping address.
type Address struct {
	ID             string `json:"id"`
	Province       string `json:"province"`
	City           string `json:"city"`
	PostalCode     string `json:"postal_code,omitempty"`
	AddressLine    string `json:"address_line"`
	Plaque         string `json:"plaque,omitempty"`
	Unit           string `json:"unit,omitempty"`
	RecipientName  string `json:"recipient_name,omitempty"`
	RecipientPhone string `json:"recipient_phone,omitempty"`
	IsDefault      bool   `json:"is_default"`
}

// CustomerStore is the persistence boundary for the customer surface.
type CustomerStore interface {
	Profile(ctx context.Context, customerID string) (*CustomerProfile, error)
	ListAddresses(ctx context.Context, customerID string) ([]Address, error)
	CreateAddress(ctx context.Context, customerID string, a Address) (string, error)
	UpdateAddress(ctx context.Context, customerID, addressID string, a Address) error
	DeleteAddress(ctx context.Context, customerID, addressID string) error
}

// DBCustomerStore implements CustomerStore.
type DBCustomerStore struct{ pool *pgxpool.Pool }

// NewDBCustomerStore wires the store.
func NewDBCustomerStore(pool *pgxpool.Pool) *DBCustomerStore {
	return &DBCustomerStore{pool: pool}
}

// Profile returns the customer row, or ErrCustomerNotFound.
func (s *DBCustomerStore) Profile(ctx context.Context, customerID string) (*CustomerProfile, error) {
	var p CustomerProfile
	var phone, email, name *string
	var verified *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, phone, email, full_name, status, phone_verified_at::text
		   FROM customers WHERE id = $1`, customerID).
		Scan(&p.ID, &phone, &email, &name, &p.Status, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCustomerNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Phone = derefStr(phone)
	p.Email = derefStr(email)
	p.FullName = derefStr(name)
	p.PhoneVerified = verified != nil
	return &p, nil
}

// ListAddresses returns the customer's addresses, default first.
func (s *DBCustomerStore) ListAddresses(ctx context.Context, customerID string) ([]Address, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, province, city, COALESCE(postal_code,''), address_line, COALESCE(plaque,''),
		        COALESCE(unit,''), COALESCE(recipient_name,''), COALESCE(recipient_phone,''), is_default
		   FROM customer_addresses WHERE customer_id = $1
		  ORDER BY is_default DESC, created_at`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Address, 0, 4)
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.Province, &a.City, &a.PostalCode, &a.AddressLine,
			&a.Plaque, &a.Unit, &a.RecipientName, &a.RecipientPhone, &a.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAddress inserts an address. When it is the default, the previous default is
// cleared in the same transaction so a customer never has two.
func (s *DBCustomerStore) CreateAddress(ctx context.Context, customerID string, a Address) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if a.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE customer_addresses SET is_default = false WHERE customer_id = $1 AND is_default`, customerID); err != nil {
			return "", err
		}
	}
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO customer_addresses (customer_id, province, city, postal_code, address_line,
		                                 plaque, unit, recipient_name, recipient_phone, is_default)
		 VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10)
		 RETURNING id`,
		customerID, a.Province, a.City, a.PostalCode, a.AddressLine, a.Plaque, a.Unit,
		a.RecipientName, a.RecipientPhone, a.IsDefault).Scan(&id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// UpdateAddress replaces the editable fields, scoped to the customer.
func (s *DBCustomerStore) UpdateAddress(ctx context.Context, customerID, addressID string, a Address) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if a.IsDefault {
		if _, err := tx.Exec(ctx,
			`UPDATE customer_addresses SET is_default = false WHERE customer_id = $1 AND is_default AND id <> $2`,
			customerID, addressID); err != nil {
			return err
		}
	}
	ct, err := tx.Exec(ctx,
		`UPDATE customer_addresses
		    SET province = $3, city = $4, postal_code = NULLIF($5,''), address_line = $6,
		        plaque = NULLIF($7,''), unit = NULLIF($8,''), recipient_name = NULLIF($9,''),
		        recipient_phone = NULLIF($10,''), is_default = $11, updated_at = now()
		  WHERE id = $1 AND customer_id = $2`,
		addressID, customerID, a.Province, a.City, a.PostalCode, a.AddressLine, a.Plaque,
		a.Unit, a.RecipientName, a.RecipientPhone, a.IsDefault)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrAddressNotFound
	}
	return tx.Commit(ctx)
}

// DeleteAddress removes an address, scoped to the customer.
func (s *DBCustomerStore) DeleteAddress(ctx context.Context, customerID, addressID string) error {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM customer_addresses WHERE id = $1 AND customer_id = $2`, addressID, customerID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrAddressNotFound
	}
	return nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
