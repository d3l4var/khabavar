-- 000012_outbox_claim.down.sql
-- Forward-only policy (DEP §۶): the down migration exists for local development only
-- and is never run by the pipeline. Dropping the lease columns re-opens the duplicate
-- dispatch window, so this is emergency-only.

BEGIN;

DROP INDEX IF EXISTS ix_domain_events_claimable;
ALTER TABLE domain_events DROP COLUMN IF EXISTS claimed_by;
ALTER TABLE domain_events DROP COLUMN IF EXISTS claimed_at;

COMMIT;
