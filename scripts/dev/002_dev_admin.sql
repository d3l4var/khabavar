-- scripts/dev/002_dev_admin.sql — DEV ONLY, never apply in production.
--
-- The baseline seed deliberately creates no staff accounts: a production install
-- provisions them out-of-band and their credentials are never committed. That means a
-- fresh database has zero admin_users, so the panel cannot be signed into at all —
-- this file fills that gap for local work, and lives outside seed/ so the embedded
-- baseline never carries a credential.
--
--   root@khabavar.ir  / Khabavar!Dev2026   SUPER_ADMIN
--   ops@khabavar.ir   / Khabavar!Dev2026   STORE_MANAGER
--   fin@khabavar.ir   / Khabavar!Dev2026   FINANCE
--
-- Apply:  psql "$KH_DB_URL" -f scripts/dev/002_dev_admin.sql

INSERT INTO admin_users (email, full_name, password_hash, status) VALUES
    ('root@khabavar.ir', 'مدیر ارشد (توسعه)', '$2a$12$fDqnlEkMtucY5ULkAF0TyeEpyRLV5XRjIWjby6J.HMguceBTBZWba', 'ACTIVE'),
    ('ops@khabavar.ir',  'مدیر فروشگاه (توسعه)', '$2a$12$fDqnlEkMtucY5ULkAF0TyeEpyRLV5XRjIWjby6J.HMguceBTBZWba', 'ACTIVE'),
    ('fin@khabavar.ir',  'مدیر مالی (توسعه)', '$2a$12$fDqnlEkMtucY5ULkAF0TyeEpyRLV5XRjIWjby6J.HMguceBTBZWba', 'ACTIVE')
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, status = 'ACTIVE';

INSERT INTO admin_user_roles (admin_user_id, role_id)
SELECT au.id, r.id
  FROM (VALUES ('root@khabavar.ir','SUPER_ADMIN'),
               ('ops@khabavar.ir','STORE_MANAGER'),
               ('fin@khabavar.ir','FINANCE')) AS v(email, role_code)
  JOIN admin_users au ON au.email = v.email
  JOIN roles r ON r.code = v.role_code
ON CONFLICT DO NOTHING;
