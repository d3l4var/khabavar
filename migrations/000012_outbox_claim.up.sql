-- 000012_outbox_claim.up.sql
-- Outbox claim lease (ADR-ARCH-007).
--
-- The original claim used `SELECT ... FOR UPDATE SKIP LOCKED` in autocommit, so the
-- row locks were released the moment the statement finished: two worker replicas
-- could claim the same event and dispatch it twice (verified by
-- TestPG_DBOutbox_ConcurrentClaimDoesNotDuplicate). A claim must be *recorded*, not
-- just locked, so the next claimer skips it until the lease expires.

BEGIN;

ALTER TABLE domain_events ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
ALTER TABLE domain_events ADD COLUMN IF NOT EXISTS claimed_by TEXT;

-- Partial index for the claim query: only unpublished rows are ever scanned.
CREATE INDEX IF NOT EXISTS ix_domain_events_claimable
    ON domain_events (occurred_at)
    WHERE published_at IS NULL;

COMMIT;
