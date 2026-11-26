-- 000011_oidc_refresh_tokens.down.sql
BEGIN;
DROP TABLE IF EXISTS oauth_refresh_tokens;
COMMIT;
