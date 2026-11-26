-- scripts/dev/002_dev_admin.sql — DEV ONLY, do not apply in production.
--
-- Gives the seeded SUPER_ADMIN a password so the panel login can be exercised
-- locally. Production staff accounts are provisioned out-of-band and their password
-- is never committed: this file lives outside seed/ on purpose, so the embedded
-- baseline (seed/001_seed_baseline.sql) never carries a credential.
--
--   email:    root@khabavar.ir      (seeded SUPER_ADMIN)
--   password: Khabavar!Dev2026      (bcrypt cost 12)
--
-- Apply:  psql "$KH_DB_URL" -f scripts/dev/002_dev_admin.sql

UPDATE admin_users
   SET password_hash = '$2a$12$fDqnlEkMtucY5ULkAF0TyeEpyRLV5XRjIWjby6J.HMguceBTBZWba',
       updated_at    = now()
 WHERE email = 'root@khabavar.ir';
