// Package commerce implements the commerce module (SAD §۴.۲, DB v2 §۱۰, R-06).
// See commerce.go for the full package documentation.
//
// Entities: carts, cart_items, orders, order_items, payment_intents,
// payment_attempts, refunds.
//
// Allowed dependencies (SAD §۴.۲): catalog, inventory, pricing, identity.
// Cross-module communication is via the internal API (method on the Service
// boundary) + outbox events only — never direct table writes (ADR-ARCH-007).
package commerce
