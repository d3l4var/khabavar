# KHABAVAR — راهنمای اجرایی پیاده‌سازی

> این سند گام‌به‌گام می‌گوید چطور پروژه را بالا بیاورید، چطور یک ماژول جدید اضافه کنید و
> چه قواعدی شکستنی‌اند. مرجع تصمیم‌ها همان اسناد R-XX است؛ این سند فقط «چطور».

---

## ۱. پیش‌نیازها

| ابزار | نسخه | چرا |
|-------|------|-----|
| Go | ۱.۲۵+ | ماژول `github.com/khabavar/khabavar` |
| PostgreSQL | ۱۶+ (روی ۱۸ هم تست شده) | مهاجرت‌ها و seed |
| `psql` | همراه PG | اعمال مهاجرت/seed |
| Node ۲۰+ | فقط برای فرانت‌اند | `web/*` |
| `golangci-lint` | اختیاری | دروازه lint |

هیچ سرویس پولی، درگاه واقعی یا پنل پیامک لازم نیست — همه Adapterها Mock-first هستند
(ADR-ARCH-011، R-19 §۷).

---

## ۲. بالا آوردن محیط محلی

```bash
# ۱) دیتابیس
createdb khabavar
export KH_DB_URL="postgres://$USER@127.0.0.1:5432/khabavar?sslmode=disable"

# ۲) مهاجرت‌ها (forward-only، به ترتیب شماره)
for f in migrations/*.up.sql; do psql "$KH_DB_URL" -v ON_ERROR_STOP=1 -q -f "$f"; done

# ۳) seed: ۱۰ نقش، ۲۴ مجوز، ۴ کلاینت OIDC، ۱۱ موضوع مجله
psql "$KH_DB_URL" -v ON_ERROR_STOP=1 -q -f seed/001_seed_baseline.sql

# ۴) باینری
go build -o bin/khabavar ./cmd/khabavar

# ۵) سه unit از همان باینری (ADR-DB-002)
KH_ROLE=identity KH_TOKEN_SIGNING_KEY=dev-key ./bin/khabavar   # :8080 — OTP + OIDC
KH_ROLE=api      KH_TOKEN_SIGNING_KEY=dev-key ./bin/khabavar   # :8080 — REST
KH_ROLE=worker   KH_TOKEN_SIGNING_KEY=dev-key ./bin/khabavar   # :8080 — outbox
```

برای اجرای هم‌زمان، `KH_HTTP_ADDR` را جدا کنید (`:8080`, `:8081`, `:8082`) یا پشت nginx
بگذارید (`deploy/nginx/khabavar.conf`).

بررسی سلامت: `curl localhost:8080/healthz` و `curl localhost:8080/readyz`.

---

## ۳. آزمون‌ها

```bash
go test ./...            # ۲۳۰+ تست، بدون نیاز به دیتابیس
go test -race ./...      # دروازه unit در CI
go vet ./... && gofmt -l .   # باید خالی باشد
```

### آزمون‌های متصل به PostgreSQL

```bash
export KH_TEST_PG_DSN="postgres://postgres@127.0.0.1:5432/khabavar_test?sslmode=disable"
go test -count=1 ./internal/...
```

- هر تست در **schema مخصوص خودش** اجرا می‌شود (`test_<random>`) و در پایان همان schema
  drop می‌شود؛ پس بسته‌ها می‌توانند موازی روی یک دیتابیس اجرا شوند.
- `migrations.ApplyAll` یک **advisory lock** می‌گیرد تا دو بسته هم‌زمان روی اشیای
  سراسری (نقش‌های 000010) به هم نزنند.
- **هرگز** این متغیر را به دیتابیس واقعی اشاره ندهید: cleanup اسکیمای تست را drop می‌کند.

---

## ۴. افزودن یک ماژول جدید

۱. پوشه `internal/modules/<name>/` با این فایل‌ها:
   - `doc.go` — مسئولیت، موجودیت‌ها، **وابستگی مجاز** (SAD §۴.۲)
   - `<name>.go` — تایپ‌های دامنه، ثابت‌های وضعیت، `Service`
   - `<name>_db.go` — `DBStore` (pgx) که اینترفیس store را پیاده می‌کند
   - `http.go` — هندلرها (فقط `net/http`، بدون روتر خارجی)
   - `<name>_test.go` + `http_test.go` — با fake store، بدون دیتابیس
۲. مهاجرت `migrations/0000NN_<name>.up.sql` + `.down.sql` (down فقط اضطراری).
۳. سرویس را در `cmd/khabavar/routes.go` داخل `services` و `mountRoutes` ثبت کنید.
۴. اگر روت ادمین دارد، **باید** در `adminPermissions` کد مجوز داشته باشد؛ وگرنه
   `apiMux.admin` در startup panic می‌کند (این عمدی است — IAM §۱).
۵. برای عملیات حساس، `AuditRecorder` را با `SetAuditor` تزریق کنید و actor را از
   `shared.ActorID(ctx)` بخوانید (IAM §۷).

### قواعد شکستنی

- هیچ ماژولی جدول ماژول دیگر را مستقیم نوشتن نمی‌کند؛ ارتباط فقط از مرز `Service` یا
  رویداد outbox.
- پول: `int64` ریال. تومان فقط در لایه نمایش.
- مجوز با **کد مجوز** چک می‌شود، هرگز با نام نقش.
- مهاجرت forward-only؛ `down` فقط با `emergency_rollback_migration` و تأیید اپراتور.
- هر POST مالی `Idempotency-Key` می‌خواهد (ADR-ARCH-008).

---

## ۵. RBAC پنل مدیریت (چطور کار می‌کند)

```
Authorization: Bearer <access-token>
        │
        ▼
authorization.Authenticate        ← identity.DBSubjectResolver
        │  (identity_sessions: token_hash, revoked_at, expires_at)
        │  توکن مشتری → 401 (IAM §۶)
        ▼
shared.WithActorID(ctx, adminID)
        ▼
authorization.RequireAudited      ← authorization.DBChecker
        │  admin_users ⋈ admin_user_roles ⋈ role_permissions ⋈ permissions
        │  admin_users.status = 'ACTIVE'
        │  رد → 403 + رکورد audit (IAM §۷)
        ▼
هندلر
```

کد مجوز هر روت در `cmd/khabavar/routes.go` → `adminPermissions` است. تست‌های
`cmd/khabavar/routes_test.go` تضمین می‌کنند:
- هر کد مجوز یکی از ۲۴ کد seed است،
- همهٔ روت‌های ادمین بدون توکن ۴۰۱ می‌دهند،
- روت ادمین بدون مجوز نگاشت‌شده در startup panic می‌کند.

### ساختن یک ادمین برای تست دستی

```sql
-- کاربر
INSERT INTO admin_users (email, full_name, status) VALUES ('ops@khabavar.ir','مدیر','ACTIVE');
-- نقش
INSERT INTO admin_user_roles (admin_user_id, role_id)
SELECT au.id, r.id FROM admin_users au, roles r
 WHERE au.email='ops@khabavar.ir' AND r.code='STORE_MANAGER';
-- نشست با توکن دلخواه (هش = HMAC-SHA256(raw, KH_TOKEN_SIGNING_KEY) به hex)
INSERT INTO identity_sessions (admin_user_id, service_code, token_hash, expires_at)
SELECT id, 'admin', '<hex-hmac>', now() + interval '1 hour'
  FROM admin_users WHERE email='ops@khabavar.ir';
```

سپس: `curl -H "Authorization: Bearer <raw-token>" localhost:8080/api/v1/admin/suppliers`

---

## ۶. outbox و worker

- ماژول‌ها رویداد را با `EventEnqueuer` در `domain_events` می‌نویسند (در همان تراکنش).
- worker با `Claim` دسته‌ای برمی‌دارد: یک `UPDATE … RETURNING` اتمیک که
  `claimed_at`/`claimed_by` را ثبت می‌کند و ردیف‌های lease‌دار را رد می‌کند.
  lease پیش‌فرض ۶۰ ثانیه است (`worker.DefaultClaimLease`) — اگر هندلری از lease
  طولانی‌تر شود، ممکن است دوبار dispatch شود؛ پس lease را بزرگ‌تر از کندترین هندلر
  نگه دارید.
- شکست → `MarkFailed` (retry_count + آزادسازی lease)؛ بعد از `maxRetries` → dead-letter.
- موفق → `MarkPublished` (پاک‌سازی claim).
- `notifications.Dispatcher` هندلر پیش‌فرض است: `OrderPaid`/`OrderShipped`/`RefundIssued`
  → پیامک به `payload.phone`؛ بدون شماره، رویداد بی‌صدا رد می‌شود.

---

## ۷. استقرار

`deploy/systemd/*.service` (سه unit) + `deploy/nginx/khabavar.conf` (چهار server block) +
`deploy/scripts/backup.sh` / `restore.sh` + `deploy/monitoring/prometheus.yml`.
رویهٔ کامل: `docs/runbooks/DEPLOYMENT.md`. متغیرها فقط از `/etc/khabavar/<unit>.env`
خوانده می‌شوند و **هرگز** در مخزن نیستند (DEP §۵).

---

## ۸. چیزهایی که هنوز نیست

فهرست کامل و به‌روز در `docs/ACTIVITY-LOG.md` §۵. خلاصه: روت‌های ادمین کاتالوگ/قیمت
طبق قرارداد، فرانت‌اند چهار اپ (فعلاً scaffold)، پوشش تست زیر آستانهٔ R-CI، و
`operationId` در قرارداد OpenAPI.
