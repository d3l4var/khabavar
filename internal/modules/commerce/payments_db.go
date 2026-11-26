package commerce

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// FindIntentByProviderReference maps a gateway transaction reference back to our payment
// intent and its order. The reference lives on payment_attempts, which is the only place
// the provider id is stored; the order comes from the intent because confirmation goes
// through ConfirmPaymentForOrder.
func (s *DBStore) FindIntentByProviderReference(ctx context.Context, reference string) (intentID, orderID string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT pa.payment_intent_id, pi.order_id
		   FROM payment_attempts pa
		   JOIN payment_intents pi ON pi.id = pa.payment_intent_id
		  WHERE pa.provider_reference = $1
		  ORDER BY pa.created_at DESC LIMIT 1`, reference).Scan(&intentID, &orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return intentID, orderID, nil
}

// OrderIDForIntent resolves a payment intent to its order. An unknown intent returns
// ("", nil) so the caller can answer a domain error rather than a 500.
func (s *DBStore) OrderIDForIntent(ctx context.Context, intentID string) (string, error) {
	var orderID string
	err := s.pool.QueryRow(ctx, `SELECT order_id FROM payment_intents WHERE id = $1`, intentID).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return orderID, nil
}
