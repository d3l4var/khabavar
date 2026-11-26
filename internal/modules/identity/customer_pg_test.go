package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// TestPG_CustomerSurface covers /me and the address book on the real schema: the profile
// read, address create/list/update/delete, the single-default rule and the scoping that
// stops one customer touching another's address.
func TestPG_CustomerSurface(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewDBCustomerStore(pool)

	var customerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO customers (phone, full_name, phone_verified_at)
		 VALUES ($1,'مشتری آزمایشی', now()) RETURNING id`,
		"+9891200"+uuid.NewString()[:6]).Scan(&customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	p, err := store.Profile(ctx, customerID)
	if err != nil || p == nil {
		t.Fatalf("Profile: %v", err)
	}
	if p.FullName != "مشتری آزمایشی" || !p.PhoneVerified {
		t.Fatalf("profile = %+v, want the seeded name and a verified phone", p)
	}
	if _, err := store.Profile(ctx, uuid.NewString()); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("unknown customer = %v, want ErrCustomerNotFound", err)
	}

	first, err := store.CreateAddress(ctx, customerID, Address{
		Province: "تهران", City: "تهران", AddressLine: "خیابان اول", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	second, err := store.CreateAddress(ctx, customerID, Address{
		Province: "اصفهان", City: "اصفهان", AddressLine: "خیابان دوم", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("CreateAddress (second): %v", err)
	}

	list, err := store.ListAddresses(ctx, customerID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListAddresses = %v, %v", list, err)
	}
	// Exactly one default: the newer one wins and the older is cleared.
	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
			if a.ID != second {
				t.Fatalf("default is %s, want the newest %s", a.ID, second)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("%d defaults, want exactly 1", defaults)
	}

	if err := store.UpdateAddress(ctx, customerID, first, Address{
		Province: "تهران", City: "کرج", AddressLine: "خیابان سوم",
	}); err != nil {
		t.Fatalf("UpdateAddress: %v", err)
	}
	list, _ = store.ListAddresses(ctx, customerID)
	for _, a := range list {
		if a.ID == first && a.City != "کرج" {
			t.Fatalf("update did not persist: %+v", a)
		}
	}

	// Another customer must not be able to touch this address.
	var otherID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO customers (phone) VALUES ($1) RETURNING id`,
		"+9891200"+uuid.NewString()[:6]).Scan(&otherID); err != nil {
		t.Fatalf("seed other customer: %v", err)
	}
	if err := store.DeleteAddress(ctx, otherID, first); !errors.Is(err, ErrAddressNotFound) {
		t.Fatalf("cross-customer delete = %v, want ErrAddressNotFound", err)
	}

	if err := store.DeleteAddress(ctx, customerID, first); err != nil {
		t.Fatalf("DeleteAddress: %v", err)
	}
	list, _ = store.ListAddresses(ctx, customerID)
	if len(list) != 1 {
		t.Fatalf("addresses = %d, want 1 after delete", len(list))
	}
}
