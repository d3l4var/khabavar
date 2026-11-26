-- 000015_idempotency.down.sql — forward-only policy (DEP §۶): emergency use only.
BEGIN;
DROP TABLE IF EXISTS idempotency_keys;
COMMIT;
