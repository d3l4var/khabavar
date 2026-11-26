-- 000014_order_number_seq.up.sql
-- The business order number (KHA-ORD-YYYY-NNNNNN, §۴.۱) is server-owned: a client
-- cannot know the sequence, so the API must mint it when the caller omits it. A
-- sequence is the only race-free source (two concurrent checkouts must not collide on
-- uq_orders_order_number).

BEGIN;

CREATE SEQUENCE IF NOT EXISTS order_number_seq START 1 INCREMENT 1;

COMMIT;
