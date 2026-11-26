# KHABAVAR — سجل فعالیت‌های توسعه

> این سند رکورد کارهایی است که روی مخزن انجام شده: چه چیزی پیدا شد، چه چیزی اصلاح شد،
> با چه شاهدی تأیید شد، و چه چیزی باقی مانده. هر ورودی باید قابل بازتولید باشد.
>
> مخزن: <https://github.com/d3l4var/khabavar> · شاخه: `main`
> دورهٔ ثبت: ۲۰۲۶-۱۱-۲۶

---

## ۱. نقطهٔ شروع

پروژه به‌صورت آرشیو تحویل داده شد، نه مخزن:

```
/home/dlp/Projects/KHABAVAR/
├── khabavar-complete.tar.gz            # کد + اسناد
├── khabavar-complete.zip
├── khabavar-full-with-binary.tar.gz
└── khabavar-linux-amd64                # باینری ساخته‌شده
```

مخزن مقصد از قبل وجود داشت (`d3l4var/khabavar`، عمومی، شاخهٔ `main`، HEAD `30fe01a`)
با ۹۰ فایل و ۱۰ کامیت `P3: ...`.

### کاری که انجام شد

1. استخراج آرشیو به `/home/dlp/Projects/KHABAVAR/khabavar`.
2. حذف `download/khabavar-repo/` — کپی کهنه و تکراری از `internal/modules` بود که
   به‌عنوان پکیج جداگانه در `go test ./...` کامپایل می‌شد (۲۴ فایل).
3. ادغام `.gitignore`: نسخهٔ آرشیو قواعد `web/*/dist`, `vendor/`, `*.env` را از دست
   داده بود؛ نسخهٔ remote حفظ و `skills/` و `__pycache__/` به آن اضافه شد.
4. `git init` + `fetch origin` + `reset origin/main` تا تاریخچهٔ موجود حفظ شود (نه
   orphan commit).
5. ۱۶ کامیت منطقی — یک ماژول در هر کامیت، به سبک خودِ مخزن، با ارجاع به بند سند.
6. push به `main` و تأیید سمت GitHub (۱۹۹ فایل، HEAD مطابق).

---

## ۲. وضعیت اولیهٔ کد (اندازه‌گیری‌شده، نه ادعا)

| معیار | مقدار |
|-------|-------|
| فایل‌های مخزن | ۱۹۹ |
| Go (prod / test) | ۱۰٫۷۹۸ / ۶٫۹۲۷ خط |
| ماژول دامنه | ۱۳ + `audit` + `shared` |
| مهاجرت | ۱۱ (بعداً ۱۲) |
| تست | ۲۱۲ تابع، همه سبز |
| `go build` / `go vet` | سبز |
| `gofmt -l .` | **۳۱ فایل قرمز** |
| پوشش تست | ۰٪ تا ۱۰۰٪، میانگین خیلی زیر آستانهٔ R-CI |

`gofmt` در کامیت `7652e1b` اصلاح شد (فقط تراز، بدون تغییر رفتار).

---

## ۳. شکاف‌هایی که در مطالعه پیدا شد

| # | شکاف | شدت | وضعیت |
|---|------|-----|-------|
| ۱ | همهٔ `/api/v1/admin/*` بدون احراز هویت mount شده بودند؛ `authorization.Require` وجود داشت ولی هیچ‌جا صدا زده نمی‌شد | بحرانی (امنیتی) | **اصلاح شد** |
| ۲ | باینری `KH_ROLE=api` اصلاً بالا نمی‌آمد: تداخل الگوی `ServeMux` | بحرانی (اجرا) | **اصلاح شد** |
| ۳ | outbox با `FOR UPDATE SKIP LOCKED` در autocommit رویداد را دوبار dispatch می‌کرد | بحرانی (داده) | **اصلاح شد** |
| ۴ | `refunds` با اسکیمای خیالی (`currency`, `processed_at`) کوئری می‌شد | بحرانی (اجرا) | **اصلاح شد** |
| ۵ | `ConfirmPaymentForOrder` هیچ `payment_attempts` نمی‌نوشت ⇒ هر بازپرداخت واقعی شکست می‌خورد | بحرانی (تجاری) | **اصلاح شد** |
| ۶ | reconciliation فرمول اشتباه داشت (هرگز true نمی‌شد) | بالا | **اصلاح شد** |
| ۷ | خطای SQL خام به کلاینت برمی‌گشت | بالا (امنیتی) | **اصلاح شد** |
| ۸ | تست‌های PG یکدیگر را می‌شکستند (یک دیتابیس مشترک + drop schema) | بالا (CI) | **اصلاح شد** |
| ۹ | ماژول `notifications` به worker وصل نبود؛ `ecosystem` mount نشده بود | متوسط | **اصلاح شد** |
| ۱۰ | `.github/workflows/ci.yml` وجود نداشت (INDEX به آن لینک می‌داد) | بالا | **اصلاح شد** |
| ۱۱ | `api/openapi.yaml` وجود نداشت (INDEX به آن لینک می‌داد) | متوسط | **اصلاح شد** |
| ۱۲ | `IMPLEMENTATION_GUIDE.md` وجود نداشت (README به آن ارجاع می‌داد) | متوسط | **اصلاح شد** |
| ۱۳ | روت‌های ادمین کاتالوگ/قیمت/audit/کاربران طبق قرارداد پیاده نشده‌اند | بالا | **باقی** |
| ۱۴ | فرانت‌اند چهار اپ فقط ۱۰ خط scaffold است | بالا | **باقی** |
| ۱۵ | پوشش تست زیر آستانهٔ R-CI (۹۰٪ مالی / ۷۵٪ سایر) | متوسط | **باقی** |
| ۱۶ | `PROJECT_MANIFEST.md` «P1–P10 کامل» می‌گفت | متوسط (صداقت گزارش) | **اصلاح شد** |

---

## ۴. اصلاحات، با شاهد

### ۴.۱ RBAC پنل مدیریت — کامیت `0a14983`

**قبل:** هر روت `/api/v1/admin/*` بدون توکن ۲۰۰ می‌داد.

**بعد:**

- `internal/modules/authorization/db.go` — `DBChecker` روی
  `admin_users ⋈ admin_user_roles ⋈ role_permissions ⋈ permissions`،
  با `status='ACTIVE'`، شناسهٔ بدشکل → `(false, nil)` (نه ۵۰۰).
- `internal/modules/authorization/middleware.go` — `Authenticate` (Bearer →
  `identity_sessions` → ctx؛ توکن مشتری → ۴۰۱) و `RequireAudited` (رد → ۴۰۳ + audit).
- `cmd/khabavar/routes.go` — جدول اعلانی `adminPermissions` (۳۳ روت) و `apiMux.admin`
  که روت بدون مجوز را در startup panic می‌کند.
- `internal/modules/shared/actor.go` — `ActorID(ctx)` تا ماژول‌ها بدانند «چه‌کسی» بدون
  import کردن `authorization` (SAD §۴.۲).

**شاهد (باینری واقعی + PG واقعی):**

```
healthz / readyz                     200 / 200
GET  /api/v1/categories              200
POST /admin/suppliers  بدون توکن     401
POST /admin/suppliers  توکن مشتری    401
POST /admin/suppliers  STORE_MANAGER 403   (supplier.manage ندارد)
GET  /admin/orders/{id}/refunds  FINANCE 403   (order.view ندارد)
POST /admin/orders/{id}/refunds  STORE_MANAGER 403   (refund انحصاری FINANCE)
POST /admin/orders/{id}/refunds  FINANCE 400   (مجوز داشت، بدنه ناقص)
```

### ۴.۲ باینری api بالا نمی‌آمد — کامیت `d2f5b6e`

`GET /orders/by-number/{number}` با `GET /orders/{id}/fulfillment` برای مسیر
`/orders/by-number/fulfillment` هر دو match می‌شد و `ServeMux` در ثبت panic می‌کرد:

```
panic: pattern "GET /api/v1/orders/by-number/{number}" ... conflicts with pattern ...
```

**اصلاح:** `/orders/{id}` حالا هم UUID و هم شمارهٔ تجاری (`KHA-ORD-YYYY-NNNNNN`) را
می‌فهمد — همان شکلی که قرارداد افشا می‌کند. جدول روت به `mountRoutes()` منتقل شد و
`TestMountRoutesHasNoPatternConflicts` کل جدول را روی `ServeMux` تازه mount می‌کند،
پس این کلاس خطا در build شکست می‌خورد نه در deploy.

### ۴.۳ outbox رویداد را دوبار dispatch می‌کرد — کامیت `0a14983`

تست هم‌روندی: `45 duplicate claims`. علت: قفل ردیف‌های `FOR UPDATE SKIP LOCKED` در
autocommit به‌محض پایان statement آزاد می‌شد.

**اصلاح:** مهاجرت `000012_outbox_claim` (`claimed_at`, `claimed_by`, ایندکس جزئی) و
`Claim` به‌صورت یک `UPDATE … RETURNING` اتمیک با lease ۶۰ ثانیه. `MarkFailed` lease را
آزاد می‌کند (retry فوری)، `MarkPublished` پاکش می‌کند.

**شاهد:** تست هم‌روندی سبز؛ worker واقعی ۵ رویداد را drain کرد و همه `published_at` گرفتند.

### ۴.۴ بازپرداخت با اسکیمای واقعی هم‌تراز شد — کامیت `d2f5b6e`

`refunds` ستون `currency` ندارد، `completed_at` دارد نه `processed_at`، و
`payment_attempt_id` اجباری است. همچنین `REFUNDED` در `chk_orders_status` نیست.

**اصلاح:** `Refund` مطابق جدول؛ statusها `REQUESTED…COMPLETED`؛ ارجاع اجباری به
attempt موفق؛ `approved_by` از actor احراز هویت‌شده؛ refund کامل → `CLOSED`؛ ثبت
audit `order.refund.issued`؛ reconciliation به `paid_total == Σ attempts` اصلاح شد.

### ۴.۵ پرداخت، attempt ثبت نمی‌کرد — کامیت `ffbe420`

بدون ردیف `payment_attempts`، `refunds.payment_attempt_id` چیزی برای ارجاع نداشت و هر
بازپرداخت واقعی با `order has no succeeded payment attempt to refund` شکست می‌خورد.

**اصلاح:** `PaymentAttempt` + `PaymentAttemptStore` + `CreateAttempt` با
`ON CONFLICT (idempotency_key) DO UPDATE … RETURNING` و کلید مشتق‌شده از intent
(`confirm:<intent-id>`)، تا webhook تکراری attempt دوم نسازد.

**شاهد — E2E کامل روی HTTP + PostgreSQL:**

```
1. POST /orders                                   → id
2. POST /orders/{id}/payment-intents               → intent id
3. POST /orders/{id}/confirm-payment               → {"status":"paid"}   (65000 = 60000+5000)
4. تکرار همان درخواست                              → {"status":"already_paid"}
5. select count(*) from payment_attempts           → 1 | SUCCEEDED
6. POST /admin/orders/{id}/refunds (FINANCE)       → refund id
7. GET  /admin/orders/{id}/refunds                 → PaymentAttemptID ✔ ApprovedBy ✔ COMPLETED
8. GET  /orders/{id}                               → Status=PAID PaidTotal=65000 RefundedTotal=20000
9. GET  /admin/orders/{id}/reconcile               → Reconciled=true NetPaid=45000
10. audit_logs                                     → order.refund.issued | 20000
11. domain_events                                  → RefundIssued
12. KH_ROLE=worker                                 → "worker: drained events=5"، همه published
```

### ۴.۶ ایزولاسیون تست PG — کامیت `0a14983`

`go test ./internal/...` با `KH_TEST_PG_DSN` شکست می‌خورد چون همهٔ بسته‌ها موازی روی
یک دیتابیس اجرا می‌شدند و cleanup هر تست `DROP SCHEMA public CASCADE` می‌زد.

**اصلاح:** `testpg.Setup` برای هر تست یک schema (`test_<random>`) می‌سازد و
`search_path` استخر را روی آن می‌گذارد؛ `ApplyAll` یک advisory lock می‌گیرد.
همچنین `SeedCustomer` predicate ایندکس جزئی را تکرار می‌کند
(`ON CONFLICT (phone) WHERE phone IS NOT NULL`) و شناسه‌های غیر-UUID در تست‌های
OIDC/outbox اصلاح شدند.

### ۴.۷ نشت خطا — کامیت `d2f5b6e`

`writeError` متن خام درایور را برمی‌گرداند:

```json
{"error":"ERROR: column \"currency\" does not exist (SQLSTATE 42703)", ...}
```

حالا ۵xx فقط `{"error":"internal error","code":"internal_error"}` می‌دهد و جزئیات در
لاگ می‌رود؛ ۴xx کد ماشین‌خوان پایدار می‌گیرد.

### ۴.۸ CI، قرارداد و راهنما — کامیت `docs/ci`

- `.github/workflows/ci.yml` — ۱۱ کار، ۷ دروازهٔ ادغام (lint، openapi-validate،
  db-migrate، unit، integration، security، a11y) + build + e2e/perf شبانه + deploy
  با environment guarded. دروازهٔ db-migrate خودش ماتریس RBAC را assert می‌کند
  (۱۰ نقش، ۲۴ مجوز، `order.refund.approve` فقط FINANCE).
- `api/openapi.yaml` — قرارداد مصوب v1.1.0 (کپی R-07) + `api/.spectral.yaml`.
  `spectral lint` → **۰ error**، ۲۹ warning.
- `IMPLEMENTATION_GUIDE.md` — راهنمای گام‌به‌گام (setup، تست، افزودن ماژول، RBAC،
  outbox، استقرار).

### ۴.۹ روت‌های خواندنی پنل طبق قرارداد — کامیت `feat(admin)`

پنج روت قراردادی که پیاده نشده بودند و فقط خواندن‌اند، اضافه شدند:

| روت | مجوز | منبع |
|-----|------|------|
| `GET /api/v1/admin/orders` | `order.view` | `commerce.OrderListStore` |
| `GET /api/v1/admin/audit-logs` | `audit.read` | `audit.ListStore` (فیلتر `action`/`actor_id`) |
| `GET /api/v1/admin/users` | `identity.user.manage` | `identity.AdminStore` (با نقش‌ها) |
| `GET /api/v1/admin/roles` | `identity.user.manage` | `identity.AdminStore` (با مجوزها) |
| `GET /api/v1/admin/permissions` | `identity.user.manage` | `identity.AdminStore` |

**شاهد — ماتریس مجوز روی باینری واقعی:**

```
route                          STORE_MANAGER  FINANCE  SUPER_ADMIN
/admin/orders                        200         403       200
/admin/audit-logs                    403         200       200
/admin/users                         403         403       200
/admin/roles                         403         403       200
/admin/permissions                   403         403       200
```

`/admin/audit-logs` رکورد واقعی بازپرداخت را برگرداند
(`order.refund.issued`، actor = شناسهٔ FINANCE، old/new values کامل).

### ۴.۱۰ سطح نوشتنی کاتالوگ و قیمت‌گذاری (P5b-admin) — کامیت `feat(admin-write)`

دوازده روت نوشتنی که قرارداد اعلام کرده بود و سرویس‌شان از قبل وجود داشت، به HTTP و
RBAC وصل شدند:

| روت | مجوز |
|-----|------|
| `POST /admin/products` | `catalog.product.edit` |
| `GET /admin/products/{id}` | `catalog.product.view` |
| `POST /admin/products/{id}/publish` | `catalog.product.publish` |
| `POST /admin/products/{id}/status` | `catalog.product.publish` |
| `POST /admin/categories` | `catalog.attribute.manage` |
| `POST /admin/variants` | `catalog.product.edit` |
| `POST /admin/variants/{id}/cost-records` | `pricing.cost.manage` |
| `GET /admin/variants/{id}/cost-records` | `pricing.cost.manage` |
| `POST /admin/variants/{id}/price-records` | `pricing.price.approve` |
| `GET /admin/variants/{id}/price-records` | `pricing.price.approve` |
| `POST /admin/price-records/{id}/approve` | `pricing.price.approve` |
| `GET /admin/variants/{id}/economics` | `reports.financial.view` |

تصمیم‌های ریزبینانه‌ای که در پیاده‌سازی گرفته شد:

- **تأییدکنندهٔ قیمت از نشست خوانده می‌شود، نه از بدنه.** `approved_by` =
  `shared.ActorID(ctx)` (R-05 §۹.۲). اگر actor خالی باشد، ۴۰۱ برمی‌گردد — fail closed.
- **`POST /admin/products/{id}/status` فقط `PAUSED` را می‌پذیرد.** انتشار روت و دروازهٔ
  جداگانه دارد، تا پنل نتواند با یک نوشتن وضعیت، MA-1/MA-6 را دور بزند.
- **دروازهٔ انتشار ۴۲۲ می‌دهد** (نه ۴۰۹ قدیمی) با کد `publish_gate_missing` و دلیل
  مشخص، به‌جای نقض خام CHECK.
- **`ErrNoValidCost` در مسیر نوشتن ۴۰۹ است، نه ۴۰۴:** نبودِ هزینهٔ معتبر یک تعارض وضعیت
  است که اپراتور باید حل کند.
- **سه دستهٔ حساس IAM §۷ حالا audit می‌شوند:** `catalog.product.published`،
  `catalog.product.status_changed`، `pricing.cost.recorded`، `pricing.price.recorded`،
  `pricing.price.approved` — با تزریق `SetAuditor` و بدون import کردن پکیج audit.
- **JSON همهٔ پاسخ‌ها snake_case شد.** `Product`, `Variant`, `Category`, `CostRecord`,
  `PriceRecord`, `UnitEconomics` تگ نداشتند و کلیدهای Go (`ID`, `SellingPrice`) برمی‌گرداندند
  — ناسازگار با بقیهٔ API و با قرارداد. یک تست قدیمی که کلید Go را assert می‌کرد به‌روز شد.

**شاهد — زنجیرهٔ کامل QC-05/R-05 روی باینری واقعی:**

```
POST /admin/products  (بدون فیلدهای دروازه)     201
POST /admin/products/{id}/publish                422  publish_gate_missing
POST /admin/products  (با sleep_relevance+spec)  201
POST /admin/products/{id}/publish                200  {"status":"PUBLISHED"}
POST /admin/variants  (SKU-MP-001)               201
POST /admin/variants  (همان SKU)                 409  duplicate_sku
POST /admin/variants/{id}/cost-records           201  total_cost=515000  currency=IRR
POST /admin/variants/{id}/price-records (450k)   201  warnings=[BR-06 …]
POST /admin/variants/{id}/price-records (900k)   201  PENDING_APPROVAL
POST /admin/price-records/{id}/approve (FINANCE) 200  approved_by=<FINANCE id>
POST /admin/price-records/{id}/approve (تکرار)   409  already_approved
GET  /admin/variants/{id}/economics              200  contribution=385000 rate=0.428
GET  /variants/{id}/price (storefront)           200  بدون فیلدهای محرمانه
GET  /admin/products/{id} (STORE_MANAGER)        200  محصول DRAFT هم دیده می‌شود
GET  /products/{id} (storefront، همان DRAFT)     404  منتشر نشده
```

ماتریس مجوز روی همین روت‌ها:

```
POST /admin/products                    STORE_MANAGER=400 (مجوز دارد)  FINANCE=403
POST /admin/variants/{id}/cost-records  STORE_MANAGER=400               FINANCE=400
POST /admin/price-records/{id}/approve  STORE_MANAGER=404               FINANCE=404
GET  /admin/variants/{id}/economics     STORE_MANAGER=403               FINANCE=200
```

(۴۰۰ = مجوز داشت، بدنه ناقص؛ ۴۰۴ = مجوز داشت، رکورد نیست. این‌ها رفتار درست‌اند.)

### ۴.۱۱ ویرایش محصول و بستن بن‌بست انتشار — کامیت `feat(catalog-update)`

هنگام اجرای E2E کاتالوگ یک **بن‌بست واقعی** پیدا شد: محصولی که بدون فیلدهای دروازه
ساخته می‌شد هرگز قابل انتشار نبود، چون هیچ راهی برای تکمیل `sleep_relevance` /
`spec_source` / `spec_last_reviewed_at` بعد از ساخت وجود نداشت. سرویس `UpdateProduct`
هم نبود.

**اصلاح:** `PATCH /api/v1/admin/products/{id}` با مجوز `catalog.product.edit`.

- **PATCH واقعی است، نه overwrite:** بدنه با pointer خوانده می‌شود تا «فیلد نیامده» با
  «فیلد خالی‌شده» قاطی نشود.
- **وضعیت از این مسیر تغییر نمی‌کند:** انتشار و توقف روت و دروازهٔ خودشان را دارند، پس
  PATCH نمی‌تواند MA-1/MA-6 را دور بزند.
- **محصول PUBLISHED نمی‌تواند فیلد دروازه را از دست بدهد** → ۴۲۲، وگرنه یا CHECK نقض
  می‌شد یا محصول زنده بی‌صدا از فروشگاه می‌افتاد.
- **دسته‌بندی‌ها فقط وقتی جایگزین می‌شوند که `category_ids` در بدنه باشد** (حتی آرایهٔ
  خالی = پاک کردن). جایگزینی در همان تراکنش انجام می‌شود.
- `catalog.product.updated` هم audit می‌شود.

**شاهد — همان محصول گیرکرده، روی باینری واقعی:**

```
select status …                        DRAFT | sleep_relevance = -
POST /admin/products/{id}/publish      422  publish_gate_missing
PATCH /admin/products/{id}             200  sleep_relevance=SLEEP_RELATED، عنوان تغییر کرد
POST /admin/products/{id}/publish      200  {"status":"PUBLISHED"}
GET  /products/{id} (storefront)       200  محصول حالا در فروشگاه دیده می‌شود
PATCH {"sleep_relevance":""}           422  (محصول منتشرشده دروازه را از دست نمی‌دهد)
PATCH {"slug":""}                      400
PATCH با توکن FINANCE                  403  (catalog.product.edit ندارد)
audit_logs                             catalog.product.updated | catalog.product.published
```

تست‌های جدید (`update_test.go`، ۶ مورد): بن‌بست انتشار، slug خالی، شکستن دروازهٔ محصول
منتشرشده، شناسهٔ ناموجود، جایگزینی صریح دسته‌بندی‌ها، و نبودِ update store.

---

## ۵. کارهای باقی‌مانده (به ترتیب اولویت)

1. **روت‌های ادمین باقی‌مانده** — کاتالوگ و قیمت‌گذاری در §۴.۱۰ بسته شد. باقی:
   `/admin/stock`، `/admin/stock/movements`، `/admin/reservations` (نوشتن موجودی از پنل؛
   سرویس inventory از قبل وجود دارد و فقط لایهٔ HTTP و مجوز `inventory.manage` لازم است)،
   (ویرایش محصول در §۴.۱۱ بسته شد).
2. **فرانت‌اند** — `web/{main,shop,magazine,admin}` هر کدام ۱۰ خط scaffold است. P2
   (Design System) و سپس صفحات UJ. تا آن زمان دروازهٔ a11y فقط هشدار می‌دهد.
3. **پوشش تست** — آستانهٔ R-CI §۳: ≥۹۰٪ منطق مالی/موجودی، ≥۷۵٪ سایر. وضعیت فعلی:
   authorization ۱۰۰٪، security ۱۰۰٪، ecosystem ۹۰٪، worker ~۷۴٪، identity ~۴۹٪،
   pricing ~۴۶٪، commerce ~۴۴٪، catalog ~۲۷٪، suppliers/inventory/fulfillment/content
   ~۲۱–۲۴٪، audit ۰٪. گام coverage در CI فعلاً `continue-on-error` است و باید بعد از
   رسیدن به آستانه blocking شود (آستانه‌ها را پایین نیاورید).
4. **`operationId` در قرارداد OpenAPI** — قرارداد مصوب v1.1.0 هیچ operationId ندارد
   (۵۸ مورد). افزودنشان یک بازبینی قرارداد (v1.2.0) با تأیید مالک است؛ تا آن زمان
   قاعده در `.spectral.yaml` خاموش است.
5. **audit برای بقیهٔ عملیات حساس** — فعلاً بازپرداخت audit می‌شود. IAM §۷ هفت دسته
   می‌خواهد: قیمت فروش، هزینه خرید، تأیید تأمین، تغییر موجودی، بازپرداخت، تغییر وضعیت
   سفارش، شرایط ارسال. الگوی تزریق `SetAuditor` آماده است.
6. **`PROJECT_MANIFEST.md`** — با وضعیت واقعی هم‌تراز شد؛ بعد از بستن موارد بالا
   دوباره به‌روزرسانی شود.

---

## ۶. بازتولید کامل تأیید

```bash
cd /home/dlp/Projects/KHABAVAR/khabavar

# ۱) استاتیک
go build ./... && go vet ./... && gofmt -l .    # همه باید پاک باشند

# ۲) واحد (بدون دیتابیس)
go test ./...

# ۳) یک کلاستر PG موقت
initdb -D /tmp/pgdata -U postgres --auth=trust
pg_ctl -D /tmp/pgdata -o "-p 55432 -k /tmp" -l /tmp/pg.log start
createdb -h 127.0.0.1 -p 55432 -U postgres khabavar_test
createdb -h 127.0.0.1 -p 55432 -U postgres khabavar

# ۴) انتگرال
KH_TEST_PG_DSN="postgres://postgres@127.0.0.1:55432/khabavar_test?sslmode=disable" \
  go test -count=1 ./internal/...

# ۵) E2E دستی
for f in migrations/*.up.sql; do psql -h 127.0.0.1 -p 55432 -U postgres -d khabavar -q -f "$f"; done
psql -h 127.0.0.1 -p 55432 -U postgres -d khabavar -q -f seed/001_seed_baseline.sql
KH_ROLE=api KH_DB_URL="postgres://postgres@127.0.0.1:55432/khabavar?sslmode=disable" \
  KH_TOKEN_SIGNING_KEY=dev-key KH_HTTP_ADDR=:18080 ./bin/khabavar
# سپس جریان سفارش→پرداخت→بازپرداخت در §۴.۵
```

---

## ۷. سجل کامیت‌ها

| کامیت | موضوع |
|-------|-------|
| `144bcfb` | `chore`: ادغام `.gitignore` |
| `1e9b3cc` | `docs`: اسناد R-01..R-CI + ۱۱ سند پایه + ۴ ران‌بوک |
| `a1c4b25` | `feat(catalog)`: درخت دسته، محصول/Variant، دروازهٔ انتشار |
| `0e40078` | `feat(pricing)`: هزینهٔ ۸ جزئی، قیمت با دروازهٔ تأیید، اقتصاد واحد |
| `cd05c3e` | `feat(commerce)`: سفارش، تأیید پرداخت، بازپرداخت، تطبیق |
| `b0bf7de` | `feat(suppliers)`: چرخهٔ عمر، چندمنبعی، ارزیابی |
| `579b12d` | `feat(inventory)`: انبار، ledger، رزرو |
| `2b2a85b` | `feat(fulfillment)`: Fulfillment، مرسوله، تایم‌لاین رویداد |
| `bf1497d` | `feat(media)`: آداپتور Storage محلی |
| `905c226` | `feat(content)`: گردش کار تحریریه، دروازهٔ انتشار |
| `fbfeb92` | `feat(notifications)`: آداپتور SMS/Email + dispatcher |
| `2bad5e0` | `feat(ecosystem)`: جستجوی یکپارچه + ناوبری مشترک |
| `9cd109c` | `feat(platform/security)`: هدرها، CORS، HSTS، CSP |
| `ff717a6` | `feat(cmd)`: سیم‌کشی ماژول‌ها در unit api |
| `83a7a26` | `deploy`: prometheus + backup/restore |
| `ec87b29` | `docs`: PROJECT_MANIFEST |
| `7652e1b` | `style`: gofmt کل درخت (۳۱ فایل) |
| `0a14983` | `feat(rbac)`: احراز هویت روت‌های ادمین + outbox lease + ایزولاسیون تست |
| `d2f5b6e` | `fix(commerce)`: هم‌ترازی refunds با اسکیما + رفع تداخل روت + نشت خطا |
| `ffbe420` | `feat(commerce)`: ثبت payment_attempts تا سفارش پرداخت‌شده قابل بازپرداخت باشد |
