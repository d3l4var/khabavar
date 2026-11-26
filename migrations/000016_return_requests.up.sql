-- 000016_return_requests: the customer's return request (R-07 §orders, MA-9).
--
-- A return request is the customer's claim; the money movement is still a refund row in
-- commerce.refunds. Keeping them apart means a request can be rejected without ever
-- touching the ledger.
CREATE TABLE return_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    customer_id     uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    -- The whole-order return is the common case; quantity lets a partial return be
    -- recorded without a second table.
    quantity        int  NOT NULL CHECK (quantity > 0),
    reason          text NOT NULL CHECK (length(btrim(reason)) > 0),
    status          text NOT NULL DEFAULT 'REQUESTED'
                    CHECK (status IN ('REQUESTED','APPROVED','REJECTED','RECEIVED','REFUNDED')),
    decision_note   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- One open request per order: a customer cannot stack duplicates while the first is
-- still being decided. The partial index keeps history open after a decision.
CREATE UNIQUE INDEX return_requests_one_open_per_order
    ON return_requests (order_id)
 WHERE status IN ('REQUESTED','APPROVED','RECEIVED');

CREATE INDEX return_requests_by_customer ON return_requests (customer_id, created_at DESC);
