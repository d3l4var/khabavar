-- scripts/dev/003_dev_oidc_redirect.sql — DEV ONLY.
--
-- The panel's OIDC redirect_uri is https://admin.khabavar.ir/auth/callback in the seed.
-- Local verification runs on http://127.0.0.1:5180, so the dev database needs that
-- origin whitelisted. Production keeps the seeded value only.
--
-- Apply:  psql "$KH_DB_URL" -f scripts/dev/003_dev_oidc_redirect.sql

UPDATE oauth_clients
   SET redirect_uris = array_append(redirect_uris, 'http://127.0.0.1:5180/auth/callback')
 WHERE client_id = 'khabavar-admin'
   AND NOT ('http://127.0.0.1:5180/auth/callback' = ANY (redirect_uris));
