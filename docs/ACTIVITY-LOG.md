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

### ۴.۱۲ پنل تأمین: موجودی، حرکت، رزرو — کامیت `feat(inventory-admin)`

سه روت قراردادی `admin-supply` اضافه شد:

| روت | مجوز | توضیح |
|-----|------|-------|
| `GET /admin/stock` | `inventory.manage` | فیلتر `warehouse_id` و `q` (SKU/عنوان)، `available = on_hand − reserved` |
| `POST /admin/stock/movements` | `inventory.adjust` | اصلاح موجودی فقط با ثبت حرکت (DB §8.3) |
| `GET /admin/reservations` | `inventory.manage` | فیلتر `status` و `expires_before`، همراه `order_number` |

**تصمیم ایمنی (مستند):** روت حرکت فقط انواع اثرگذار بر `on_hand` را می‌پذیرد
(`RECEIPT`, `RETURN`, `ADJUSTMENT`, `DAMAGE`, `TRANSFER`). انواع `RESERVATION`،
`RELEASE` و `SALE` روی `reserved_quantity` اثر می‌گذارند و متعلق به جریان سفارش‌اند؛
اجازه دادنشان از پنل، ناوردای `available = on_hand − reserved` را خراب می‌کرد (DB §8.2).
این سه نوع با ۴۰۰ و پیام روشن رد می‌شوند.

**دو باگ که همین E2E پیدا کرد:**

1. کوئری موجودی از `w.code` استفاده می‌کرد؛ ستون واقعی `warehouse_code` است → هر
   `GET /admin/stock` با ۵۰۰ می‌افتاد.
2. موجودی منفی ۵۰۰ برمی‌گرداند، چون `ErrNegativeOnHand` در نگاشت خطا نبود؛ حالا ۴۰۹
   با کد `insufficient_stock` است.

همچنین صفحهٔ خالی حالا `items: []` می‌دهد نه `null` (قرارداد آرایه اعلام کرده).

**شاهد:**

```
POST /admin/stock/movements RECEIPT +10 (SUPER_ADMIN)   201
POST /admin/stock/movements DAMAGE  -3                  201
POST /admin/stock/movements ADJUSTMENT -999             409  insufficient_stock
POST /admin/stock/movements RESERVATION +1              400  movement type not allowed
POST /admin/stock/movements با STORE_MANAGER            403  (inventory.adjust ندارد)
GET  /admin/stock?q=SKU-MP                              200  on_hand=7 available=7
GET  /admin/reservations                                200  items=[] page.total=0
GET  /admin/reservations?expires_before=not-a-date       400
stock_movements                                         2 ردیف (رد شدن‌ها چیزی ننوشت)
```

### ۴.۱۳ بازبینی روند توسعه — دو ایراد در ابزار خودمان

پیش از ادامه، کل مسیر از ابتدا بازبینی شد (build/vet/gofmt، ۲۳۶ تست، کلاستر PG تازه،
اعمال ۱۳ مهاجرت + seed، ماتریس RBAC). دو ایراد در **ابزار تأیید** پیدا شد، نه در محصول:

1. **assertion دروازهٔ `db-migrate` غلط بود.** نوشته بودم `order.refund.approve` نباید به
   هیچ نقشی جز FINANCE داده شود — اما SUPER_ADMIN طبق IAM §۴ هر ۲۴ مجوز را دارد. یعنی
   دروازه روی یک seed **درست** قرمز می‌شد. اصلاح شد (`NOT IN ('FINANCE','SUPER_ADMIN')`)
   و یک assert دوم اضافه شد که SUPER_ADMIN دقیقاً ۲۴ مجوز داشته باشد.
2. **ماتریس RBAC هیچ تستی نداشت** — فقط با psql دستی چک شده بود. حالا `seed/embed.go` +
   `seed/seed_test.go` با ۵ تست: شمارش‌ها (۱۰ نقش/۲۴ مجوز/۴ کلاینت/۱۱ موضوع)،
   SUPER_ADMIN = ۲۴، انحصار بازپرداخت، هشت نقش با must/must-not، و جداسازی scope در OIDC.
   کار `integration` در CI هم از `./internal/...` به `./...` تغییر کرد تا این گیت اجرا شود.

### ۴.۱۴ نشت محصول منتشرنشده در فروشگاه — کامیت `fix(catalog-visibility)`

اولین تست انتگرالِ `DBStore` کاتالوگ (`catalog_pg_test.go`) یک **نشت واقعی** پیدا کرد:
`GetProductDetail` هیچ فیلتری روی وضعیت ندارد — که برای پنل درست است — ولی همان متد
پشت روت عمومی `GET /api/v1/products/{id}` هم بود. یعنی هر کسی با داشتن UUID می‌توانست
محصول `DRAFT`/`PAUSED`/`DISCONTINUED` را بخواند (R-04 §۵، QC-05).

**اصلاح:** متد جداگانهٔ `GetStorefrontProduct` که فقط `PUBLISHED` را برمی‌گرداند و
در غیر این صورت `ErrProductNotFound` می‌دهد؛ روت عمومی از آن استفاده می‌کند و روت پنل
همان `GetProductDetail` می‌ماند.

**شاهد (باینری واقعی):**

```
POST /admin/products (DRAFT)            201
GET  /products/{id}  (فروشگاه)          404
GET  /admin/products/{id} (پنل)         200
GET  /products/{id}  (محصول PUBLISHED)  200
```

> تصحیح سجل: در §۴.۱۰ برای یک محصول DRAFT پاسخ ۴۰۴ فروشگاه ثبت شده بود، اما آن یک
> مشاهدهٔ موردی بود نه یک قاعدهٔ تضمین‌شده — کد فیلتری نداشت. حالا قاعده صریح، تست‌شده
> (`TestStorefrontDetailHidesUnpublished` + تست PG) و روی باینری تأیید شده است.

همچنین اولین تست انتگرال کاتالوگ اضافه شد (`catalog_pg_test.go`): چرخهٔ کامل دسته →
محصول → انتشار → Variant → جستجو → توقف، به‌علاوهٔ اثبات اینکه قید
`chk_products_publish_gates` در سطح دیتابیس هم انتشار بدون دروازه را رد می‌کند.

### ۴.۱۵ تست انتگرال DBStore برای چهار ماژول — کامیت `test(pg-store)`

پوشش تست واحد با fake store اندازه‌گیری می‌شد، پس کد `DBStore` (جایی که هر دو باگ
اسکیمایی رخ داد) عملاً تست نشده بود. چهار فایل تست انتگرال اضافه شد:

| ماژول | تست‌ها | چه چیزی را قفل می‌کند |
|-------|--------|------------------------|
| pricing | ۵ | هشت جزء هزینه، دروازهٔ تأیید قیمت، پنجرهٔ اعتبار، BR-06، اقتصاد واحد |
| suppliers | ۳ | ماشین حالت ۸ وضعیتی، یکتایی (تأمین‌کننده، محصول)، تاریخچهٔ قیمت، ارزیابی وزنی |
| inventory | ۴ | ناوردای `available = on_hand − reserved`، oversell، convert، منبع رزرو، sweep انقضا |
| fulfillment | ۴ | ماشین حالت، قاعدهٔ منبع، ارسال چندبسته‌ای، تایم‌لاین، سقف تخصیص |

**دو نقص واقعی که همین تست‌ها پیدا کردند:**

1. **تاریخچهٔ قیمت تأمین‌کننده ناقص بود.** `CreateSupplierVariant` قیمت اولیه را در
   `supplier_variant_price_history` ثبت نمی‌کرد، پس بازسازی تایم‌لاین قیمت از تاریخچه
   غیرممکن بود (§7.3: هر قیمت باید قابل ردیابی باشد). حالا در همان تراکنش ثبت می‌شود.
2. **خطای ثبت رویداد ارسال بی‌صدا بلعیده می‌شد.** `TransitionShipmentStatus` مقدار
   `_, _ =` داشت؛ اگر `source` مجاز نبود (CHECK فقط `MANUAL`/`CARRIER_API`/`WEBHOOK`)،
   وضعیت مرسوله عوض می‌شد ولی رویدادی در تایم‌لاین نمی‌ماند. حالا `source` **قبل** از
   تغییر وضعیت اعتبارسنجی می‌شود و خطای ثبت رویداد برگردانده می‌شود.

**پوشش (با PG، قبل → بعد):**

```
pricing      45.5% → 61.7%      catalog     26.8% → 62.0%
suppliers    23.1% → 60.1%      inventory   23.3% → 44.3%
fulfillment  24.1% → 52.2%
```

گام coverage در CI از کار `unit` به کار `integration` منتقل شد — چون تست‌های DBStore
همان‌جا اجرا می‌شوند و اندازه‌گیری بدون دیتابیس عدد fake-store می‌داد. آستانه‌ها
(۹۰٪ مالی / ۷۵٪ سایر) دست‌نخورده‌اند و گام هنوز `continue-on-error` است.

### ۴.۱۶ تست انتگرال commerce و content — کامیت `test(pg-store-2)`

دو ماژول باقی‌ماندهٔ بدون تست `DBStore` پوشش داده شدند (۸ تست جدید):

| ماژول | چه چیزی قفل شد |
|-------|-----------------|
| commerce | ساخت سفارش با محاسبهٔ مبالغ (۲×۵۰۰۰۰+۱۰۰۰۰=۱۱۰۰۰۰)، یکتایی شمارهٔ تجاری، یافتن با UUID و با شماره، قواعد لغو، رویدادهای outbox، فهرست پنل با فیلتر وضعیت، قید `chk_orders_totals` |
| content | گردش کار تحریریه، **دروازهٔ انتشار** (بدون منبع → رد و وضعیت تغییر نمی‌کند)، رابطهٔ مقاله-محصول با دلیل اجباری، زنجیرهٔ ترجمه، انتشار زمان‌بندی‌شده |

**یک نقص واقعی:** قید MA-5 در دیتابیس نبود. ستون `relation_reason TEXT NOT NULL` بود،
یعنی رشتهٔ خالی را می‌پذیرفت؛ قاعده فقط در سرویس اعمال می‌شد و یک نوشتن مستقیم می‌توانست
«دلیل ارتباط» را به یک فیلد تشریفاتی تبدیل کند. مهاجرت `000013_relation_reason_check`
اضافه شد با `CHECK (btrim(relation_reason) <> '')` — همان الگوی
`chk_products_publish_gates`.

**پوشش (با PG):** commerce ۴۴٪ → ۶۶.۹٪ · content ۲۱.۵٪ → ۴۴.۴٪

### ۴.۱۷ شروع فرانت‌اند: Design System + اپ `main` — کامیت `feat(web-main)`

تا اینجا هر چهار اپ ۱۰ خط scaffold بودند. اکنون:

**`web/shared/` — Design System مشترک (P2)**
- `tokens.css`: پالت **مصوب** برند (Midnight `#10172B`، Deep Navy، Sleep Blue، Lavender،
  Moon White، Mist، Charcoal، Success/Warning/Error) به‌صورت CSS custom property،
  به‌علاوهٔ مقیاس فاصله/شعاع/سایه/تایپوگرافی. کامپوننت‌ها فقط از توکن استفاده می‌کنند —
  قاعدهٔ سند برند («استفادهٔ مستقیم از کد رنگ ممنوع»).
- `components.tsx`: `Container`, `Button`, `Badge`, `Card`, `Grid`, `Header`, `Footer`,
  `ProductCard`, `ArticleCard`, `StateBlock` (سه حالت مشترک Loading/Empty/Error) و
  `useAsync` که همان سه حالت را یک‌جا مدیریت می‌کند.
- `api.ts`: کلاینت تایپ‌شدهٔ API با پاکت خطای یکسان، به‌علاوهٔ `formatToman` (تبدیل ریال→تومان
  فقط در لایهٔ نمایش، ADR-DATA-001).

**`web/main` — سایت اصلی**
- روتر hash بدون هیچ وابستگی جدید (لیست deps عمداً کوچک است).
- صفحه‌ها: خانه (مقالات + محصولات)، فروشگاه با جست‌وجو، جزئیات محصول، مجله، جزئیات مقاله،
  جست‌وجوی یکپارچه (product+article از ماژول ecosystem).
- RTL بومی (`dir="rtl"`, `lang="fa-IR"`, ویژگی‌های منطقی CSS).

**زیرساخت:** `web/package.json` با npm workspaces (`shared`, `main`, `shop`, `magazine`,
`admin`) تا `web/shared` بتواند `react` را از store مشترک resolve کند — بدون آن، tsc و
Vite فایل‌های بیرون از ریشهٔ اپ را نمی‌توانستند type-check/باندل کنند.

**دو ایراد در scaffold که اصلاح شد:**
1. `tsconfig.json` گزینهٔ نامعتبر `"rtl": true` داشت (tsc چنین گزینه‌ای ندارد) → حذف شد.
2. اسکریپت build `tsc -b` بود که فایل `.js` را **کنار سورس** منتشر می‌کرد
   (`src/main.js`). به `tsc --noEmit && vite build` تغییر کرد و `*.tsbuildinfo` به
   `.gitignore` رفت.

**شاهد:**

```
npm run build (web/main)      tsc --noEmit + vite build ✓
                              dist/assets/index-*.js 156.69 kB │ gzip 49.89 kB
dev server                    /  → 200، <html lang="fa-IR" dir="rtl">
                              /src/main.tsx → 200
                              /@fs/.../shared/components.tsx → 200
                              /@fs/.../shared/tokens.css → 200
پروکسی API                    /api/v1/navigation → 200 (payload واقعی ecosystem)
                              /api/v1/articles   → 200
```

> این محدودیت در §۴.۱۸ رفع شد: Lightpanda نصب و اپ در مرورگر واقعی تأیید شد.

### ۴.۱۸ تأیید فرانت‌اند در مرورگر واقعی + دو باگ — کامیت `fix(api): empty lists`

**Lightpanda نصب شد** (`~/.local/bin/lightpanda`, v1.1.0-nightly) و اپ `main` در یک
مرورگر واقعی اجرا و بازرسی شد. سه چیز روشن شد:

1. **Lightpanda اسکریپت `type="module"` را اجرا نمی‌کند** — اپ با باندل ESM اصلاً رندر
   نمی‌شد. برای تأیید، یک باندل IIFE با esbuild ساختم (`dist-lp/`، فقط برای بازرسی،
   در `.gitignore`) و همان اپ ۱۰۰٪ رندر شد. **این محدودیت مرورگر است، نه باگ محصول:**
   باندل تولیدی همان ESM استاندارد می‌ماند که مرورگرهای واقعی اجرا می‌کنند.

2. **`"data": null` به‌جای `[]`** — همهٔ endpointهای فهرستی وقتی نتیجه خالی بود `null`
   برمی‌گرداندند، چون slice تهی در Go به JSON `null` تبدیل می‌شود. قرارداد آرایه اعلام
   کرده و کلاینت هم روی `.length` می‌شکست (خطای واقعی: `Cannot read properties of null
   (reading 'length')`). **۲۳ نقطه در ۸ فایل `*_db.go`** اصلاح شد و کلاینت هم
   defensively نوشته شد.

3. **JSON چند ساختار Go-style بود** — `StorefrontPrice`، `OrderInput`، `OrderItemInput`
   تگ نداشتند. snake_case شدند تا قرارداد ساخت سفارش از سمت کلاینت قابل استفاده باشد.

**شاهد — اجرای واقعی در Lightpanda:**

```
errs: []                        root_len: 5132
h1:  خواب بهتر، از آگاهی شروع می‌شود
h2:  تازه‌ترین مقالات | محصولات        cards: 3
nav: #/ · #/shop · #/magazine · #/search + حساب من/مدیریت (خارجی، از ecosystem)
/articles خالی →  «هنوز مقاله‌ای منتشر نشده است.»  (حالت Empty درست)
#/shop        → فروشگاه، ۳ کارت، input جست‌وجو
#/magazine    → مجله
#/search      → جست‌وجو
#/product/…   → بالش طبی مموری + «کد کالا: SKU-MP-001» (دادهٔ واقعی API)
#/nope        → صفحهٔ «پیدا نشد»
```

### ۴.۱۹ اپ `shop` و مسیر خرید کامل در مرورگر — کامیت `feat(web-shop)`

**`web/shop`** ساخته شد: فهرست محصولات با جست‌وجو، صفحهٔ محصول با **قیمت زندهٔ مصوب**
(`GET /variants/{id}/price`)، سبد سمت کلاینت (localStorage — بک‌اند cart ندارد)، تسویه و
صفحهٔ تأیید سفارش.

**قاعدهٔ مهم در تسویه:** قیمت هر قلم **دوباره** از API خوانده می‌شود، نه از سبد؛ یک تب
کهنه نمی‌تواند قیمت قدیمی را ثبت کند.

**سه باگ واقعی که همین مسیر در مرورگر پیدا کرد:**

1. **شمارهٔ سفارش اجباری بود، در حالی که مالک آن سرور است.** `POST /orders` بدون
   `order_number` با خطای `commerce: order_number is required` رد می‌شد — یعنی کلاینت وب
   مجبور بود شمارهٔ ترتیبی سرور را از خودش بسازد. مهاجرت `000014_order_number_seq`
   (سکانس)، متد `NextOrderNumber` در `Store`، و تولید خودکار در `CreateOrder` **قبل از**
   اعتبارسنجی. تست واحد قدیمی که «رد شدن» را انتظار داشت با تست «تولید شدن» جایگزین شد.
2. **`Order`/`OrderItem` تگ JSON نداشتند** — پاسخ `/orders/{id}` کلیدهای Go
   (`OrderNumber`, `GrandTotal`) برمی‌گرداند و صفحهٔ تأیید «ناعدد تومان» نشان می‌داد.
   snake_case شدند.
3. **`GET /articles` خالی `null` می‌داد** (در §۴.۱۸ اصلاح شد).

**شاهد — مسیر خرید کامل در Lightpanda (مرورگر واقعی، بدون خطا):**

```
product   → بالش طبی مموری | ۹۰٬۰۰۰ تومان
add       → #/cart | ۱ قلم در سبد | localStorage نوشته شد
checkout  → فرم پر شد (۰۹۱۲۰۰۰۰۰۹۹ / تهران / تهران)
submit    → #/order/77ce29a0-…
تأیید     → شمارهٔ سفارش: KHA-ORD-2026-000002 | وضعیت: PENDING_PAYMENT | مبلغ: ۹۰٬۰۰۰ تومان
errs: []
```

### ۴.۲۰ اپ `admin` + روت فهرست محصولات — کامیت `feat(web-admin)`

**شکاف بک‌اند:** قرارداد `GET /api/v1/admin/products` را داشت ولی پیاده نشده بود، پس پنل
هیچ راهی برای دیدن پیش‌نویس‌ها نداشت (جست‌وجوی عمومی فقط `PUBLISHED` می‌دهد). افزوده شد:
`catalog.AdminListStore.List` با فیلتر وضعیت/عبارت و ستون محاسبی
`publish_gate_satisfied`، روت با مجوز `catalog.product.view` (شمار روت‌های ادمین: ۵۸).

**`web/admin`** ساخته شد: دروازهٔ توکن، سفارش‌ها (با فیلتر وضعیت)، محصولات (تکمیل دروازهٔ
انتشار + انتشار)، انبار (موجودی + ثبت حرکت)، Audit، نقش‌ها.

**اصل طراحی:** پنل **هیچ‌گاه خودش مجوز تعیین نمی‌کند** — هر روت در API با
`authorization.Require` و ماتریس seed بررسی می‌شود و ۴۰۳ عیناً به کاربر نشان داده
می‌شود. این‌طور منبع حقیقت یکی می‌ماند (IAM §۱) و ماتریس در کلاینت تکرار نمی‌شود.

**شاهد — پنل در Lightpanda، با توکن SUPER_ADMIN:**

```
بدون توکن  → /admin/products 401، همهٔ پنج روت با توکن 200
ورود       → «سفارش‌ها» | ۴ سفارش واقعی (KHA-ORD-2026-000001/000002 · ۹۰٬۰۰۰ تومان)
محصولات    → ۴ کارت؛ پیش‌نویس: DRAFT + «دروازهٔ انتشار ناقص» + دکمهٔ تکمیل دروازه
انتشار     → «محصول منتشر شد.» → کارت به PUBLISHED تغییر کرد
errs: []
```

**باگ هارنس (نه محصول):** پروکسی تست فقط `GET/POST` را فوروارد می‌کرد و `PATCH` را با
**501** رد می‌کرد؛ پس دکمهٔ «تکمیل دروازه» در مرورگر کار نمی‌کرد. `do_PATCH/PUT/DELETE`
اضافه شد. (تأیید با `curl` مستقیم: PATCH → 200.)

### ۴.۲۱ ورود واقعی پنل: ایمیل + گذرواژه — کامیت `feat(identity): staff login`

**شکاف:** `admin_users.password_hash` از مهاجرت اول وجود داشت ولی استفاده نمی‌شد
(«نسخه اول: ورود پنل با OTP؛ فیلد برای آینده»)، و `OTPVerify` فقط نشست **مشتری** صادر
می‌کرد — یعنی هیچ راهی برای ورود کارمند به پنل نبود؛ پنل مجبور بود توکن دستی بگیرد.

**افزوده شد:**
- `Service.AdminLogin` با **bcrypt (cost 12)** — و برای ایمیل ناشناخته هم یک مقایسهٔ
  bcrypt روی هش ساختگی اجرا می‌شود تا زمان پاسخ وجود حساب را لو ندهد (بدون enumeration).
- `Service.IssueAdminSession` + `Session.AdminUserID` + درج `admin_user_id` در
  `identity_sessions` (نشست کارمند هرگز `customer_id` ندارد).
- `POST /api/v1/auth/admin/login` روی واحد identity.
- `scripts/dev/002_dev_admin.sql` — **بیرون از `seed/`** تا baseline جاسازی‌شده هرگز
  گذرواژه حمل نکند؛ فقط برای محیط توسعه.

**شاهد — API:**

```
root@khabavar.ir + گذرواژهٔ درست → 200 {access_token}
گذرواژهٔ نادرست                  → 401
ایمیل ناشناخته                   → 401   (همان پاسخ، بدون تفکیک)
```

**شاهد — پنل در Lightpanda:**

```
فرم ورود  → email,password
ورود      → «سفارش‌ها» | ۴ سفارش | توکن در sessionStorage
errs: []
```

**باقی‌مانده:** تبادل کد OIDC در مرورگر (Authorization Code + PKCE سمت کلاینت). سرور
`/oauth/authorize` و `/oauth/token` را دارد و تست‌شده است؛ کلاینت وب فعلاً مستقیم وارد
می‌شود.

### ۴.۲۲ اپ `magazine` + فیلتر موضوع — کامیت `feat(web-magazine)`

**دو شکاف قبل از فرانت:**
1. `Article`/`Topic`/`ArticleSource` تگ JSON نداشتند — `/articles` کلیدهای Go
   (`Title`, `Slug`, `PublishedAt`) برمی‌گرداند. snake_case شدند.
2. `GET /api/v1/articles` فیلتر موضوع نداشت، پس صفحهٔ موضوع مجبور بود در کلاینت
   فیلتر کند (گمراه‌کننده). پارامتر `?topic=<id>` اضافه شد و به
   `ListArticlesByTopic` (که وجود داشت ولی روت نداشت) وصل شد.

**`web/magazine`** ساخته شد: خانه (موضوع‌ها + تازه‌ترین مقالات)، صفحهٔ مقاله
(`#/article/{slug}`)، صفحهٔ موضوع (`#/topic/{id}`)، فهرست موضوع‌ها.

**نکتهٔ امنیتی:** بدنهٔ مقاله به‌صورت **متن ساده** و پاراگراف‌به‌پاراگراف رندر می‌شود،
نه HTML — ویرایشگر نمی‌تواند مارک‌آپ به صفحهٔ عمومی تزریق کند.

**شاهد — زنجیرهٔ کامل محتوا (API):**

```
POST /admin/articles (بدون منبع) → publish 409  ← دروازهٔ ADR-CONTENT-001
POST /admin/articles/{id}/sources (GUIDELINE) → 201
submit 200 → approve 200 → publish 200
GET /articles                → مقاله با کلیدهای snake_case
GET /articles?topic=<id>     → همان مقاله در موضوع «علوم خواب»
GET /articles/{slug}         → 200
```

**شاهد — مجله در Lightpanda:**

```
خانه      → «مجلهٔ خواب» | ۶ چیپ موضوع | ۱ کارت مقاله | «۵ آذر ۱۴۰۵»
مقاله     → h1 + «۵ آذر ۱۴۰۵ · ORIGINAL» + خلاصه + بدنه (۳ پاراگراف)
موضوع     → «علوم خواب» | ۱ کارت
موضوع‌ها  → ۱۱ موضوع
errs: []
```

### ۴.۲۳ صفحهٔ محصول در پنل: ویرایش، گزینه، زنجیرهٔ قیمت — کامیت `feat(admin-product)`

**شکاف بک‌اند:** `GET /admin/products/{id}` وضعیت دروازهٔ انتشار را برنمی‌گرداند، پس پنل
مجبور بود قاعدهٔ MA-1/MA-6 را در کلاینت بازسازی کند. `ProductDetail.PublishGateSatisfied`
اضافه شد و سرور همان قاعده‌ای را که `PublishProduct` اعمال می‌کند گزارش می‌دهد.

**`web/admin` — صفحهٔ `#/product/{id}`:** ویرایش فیلدها (PATCH)، فهرست گزینه‌ها، افزودن
گزینه، ثبت بهای تمام‌شده، ثبت قیمت، تأیید قیمت، انتشار.

**باگ واقعی در کلاینت:** جدول `price_records` ستون `status` **ندارد** — تأیید با
`approved_by` بیان می‌شود. پنل همهٔ رکوردها را «در انتظار تأیید» نشان می‌داد و دکمهٔ
تأیید را برای رکورد تأییدشده هم می‌آورد. حالا از `approved_by` مشتق می‌شود.

**شاهد — زنجیرهٔ کامل روی محصول پیش‌نویس:**

```
افزودن گزینه از پنل   → SKU-DRAFT-01 (DRAFT)
ثبت بهای تمام‌شده     → 201
ثبت قیمت ۹۰۰٬۰۰۰      → PENDING_APPROVAL
تأیید قیمت            → 200
قیمت فروشگاه          → {"selling_price":900000,"currency":"IRR"}
پنل                   → «۹۰٬۰۰۰ تومان APPROVED» و «۹۰٬۰۰۰ تومان PENDING_APPROVAL» + یک دکمهٔ تأیید
نشان دروازه           → PUBLISHED | دروازهٔ انتشار کامل (دکمهٔ انتشار پنهان)
errs: []
```

### ۴.۲۴ ایجاد محصول + دسته‌بندی + یک باگ نوشتن خاموش — کامیت `fix(catalog): category write`

**شکاف seed:** جدول `categories` **خالی** بود — ۱۱ موضوع مجله seed شده بود ولی هیچ
دستهٔ محصولی نه. یعنی محصول قابل دسته‌بندی نبود و فروشگاه ناوبری نداشت. درخت دو سطحی
(۴ دستهٔ اصلی + ۶ زیردسته) با `ON CONFLICT (slug) DO NOTHING` به seed اضافه شد.

**`web/admin`:** فرم «محصول جدید» (عنوان/اسلاگ خودکار/خلاصه) که محصول DRAFT می‌سازد و
به صفحهٔ جزئیات می‌رود، و انتخابگر دسته‌بندی (درخت عمومی، انتخاب فعلی از خود محصول
بارگذاری می‌شود تا اپراتور دسته‌های نادیده را پاک نکند).

**باگ نوشتن خاموش (جدی):** `Service.UpdateProduct` مجموعهٔ دسته‌ها را **بعد** از فراخوانی
store به `p.CategoryIDs` نسبت می‌داد، ولی store همان `p` را می‌گیرد و از رویش
`DELETE`+`INSERT` می‌کند. نتیجه: لینک‌ها پاک می‌شدند، هیچ‌چیز درج نمی‌شد، API **۲۰۰**
برمی‌گرداند و پاسخ مجموعهٔ جدید را echo می‌کرد — یعنی پاسخ دروغ می‌گفت.

```
قبل از رفع:  PATCH {"category_ids":[X]} → 200 ، پاسخ category_ids:[X] ، DB rows = 0
بعد از رفع:  PATCH {"category_ids":[X]} → 200 ، DB rows = 1
```

تست رگرسیون `TestUpdateProduct_PersistsCategoryReplacement` اضافه شد (روی store حافظه‌ای
همان نشت را می‌گیرد).

**شاهد — چرخهٔ کامل از پنل در Lightpanda:**

```
محصول جدید      → «پتوی وزن‌دار» → #/product/38d5651c-… (DRAFT، دروازهٔ ناقص)
دسته‌بندی‌ها      → ۱۰ چیپ با تورفتگی درخت (تشک و تخت / — تشک بادی / …)
ذخیرهٔ دسته‌بندی  → ۲۰۰ و ردیف در product_categories
errs: []
```

### ۴.۲۵ بازبینی روند + دو یافتهٔ تازه — کامیت `fix(silent-writes)`

**بازبینی (همه با خروجی واقعی):**

```
HEAD == origin/main == 90781af · tree پاک · ۵۱ کامیت
go build / go vet / gofmt -l  → پاک
go test ./...                → ۱۶ بسته ok (با و بدون PG)
build هر چهار اپ            → main, shop, admin, magazine
بازتولید از صفر (کلاستر تازه، پورت 55434):
  ۱۴ مهاجرت بدون خطا · seed بدون خطا · ۵۳ جدول
  roles=10 perms=24 oauth_clients=4 topics=11 categories=10
```

**یافتهٔ ۱ — نصب تازه هیچ کاربر کارمندی ندارد.** baseline seed عمداً حساب کارمند
نمی‌سازد (گذرواژه نباید در مخزن باشد)، ولی نتیجه این بود که `admin_users` روی دیتابیس
تازه **صفر** است و پنل اصلاً قابل ورود نیست؛ `scripts/dev/002_dev_admin.sql` هم فقط
`UPDATE` می‌کرد روی ردیفی که وجود نداشت (بی‌اثر). حالا همان فایل سه حساب توسعه را
`INSERT` می‌کند (root/ops/fin با نقش‌های SUPER_ADMIN/STORE_MANAGER/FINANCE) و idempotent
است. شاهد روی دیتابیس تازه: ورود root و ops → **200**.

**یافتهٔ ۲ — همان کلاس «نوشتن خاموش» در دو مسیر دیگر.** سه `UPDATE` بدون بررسی
`RowsAffected` روی ردیف ناموجود «موفق» گزارش می‌شدند:

- `UpdateOrderStatus` و `UpdateOrderPaid` (پول!) → حالا `ErrOrderNotFound`
- `UpdateProductStatus` (انتشار/توقف) → حالا `ErrProductNotFound`

تست‌های PG: `TestPG_StatusUpdatesRejectUnknownOrder` و
`TestPG_StatusUpdateRejectsUnknownProduct` — هر دو PASS.

**بازرسی الگو:** تنها یک مسیر با پارامتر `replace… bool` وجود داشت (همان باگ دسته‌بندی،
رفع‌شده). ۱۰ `UPDATE` دیگر بدون بررسی باقی مانده که همه داخل تراکنش با
`SELECT … FOR UPDATE` قبلی هستند (انبار، OAuth) — ریسک پایین، در فهرست کارها ثبت شد.

### ۴.۲۶ ورود مرورگری OIDC + دو باگ مسدودکنندهٔ ورود — کامیت `feat(oidc-login)`

**شکاف:** `/oauth/authorize` بدون نشست به `GET /api/v1/auth/otp/login?continuation=…`
رجوع می‌داد که **mount نشده بود** — یعنی هیچ کلاینت ثالثی نمی‌توانست جریان را در مرورگر
تمام کند (dead-end واقعی).

**افزوده شد:** صفحهٔ ورود سمت‌سرور (HTML ساده، بدون فریم‌ورک) + `POST` که کد را
می‌سنجد، نشست می‌سازد، `continuation` را با HMAC بازمی‌خواند و جریان را ادامه می‌دهد.
توکن نشست هرگز در URL نمی‌رود.

**دو باگ مسدودکننده که همین مسیر بیرون کشید:**

1. **`OTPVerify` مقدار پیش‌فرض purpose را اعمال نمی‌کرد.** `OTPStart` خالی را `LOGIN`
   می‌گیرد ولی verify نه؛ کلاینتی که در فراخوانی دوم `purpose` را نمی‌فرستاد، چالش را با
   `purpose=''` می‌جست، پیدا نمی‌کرد و پاسخ **«invalid code»** می‌گرفت — که مثل کد اشتباه
   به نظر می‌رسد، نه مثل فیلد جاافتاده.
2. **ورود مشتری روی schema واقعی کاملاً شکسته بود.** `UpsertCustomerByPhone` از
   `ON CONFLICT (phone)` استفاده می‌کرد، ولی ایندکس یکتا **جزئی** است
   (`uq_customers_phone … WHERE phone IS NOT NULL`). Postgres بدون تکرار همان شرط، ایندکس
   را نمی‌شناسد → `42P10` → هر ورود با OTP در آخرین قدم ۵۰۰ می‌داد. تست‌های واحد با
   store حافظه‌ای این را نمی‌دیدند.

**شاهد — جریان کامل با curl:**

```
GET /oauth/authorize (بدون نشست) → 302 به صفحهٔ ورود (قبلاً 404)
GET  /api/v1/auth/otp/login       → 200 (HTML)
POST /api/v1/auth/otp/login       → {"redirect_url":"…?code=932b225e…&state=xyz"}
POST /oauth/token (code+PKCE)     → {"access_token":"df328d27…","token_type":"Bearer","expires_in":900,"refresh_token":"8cd82907…"}
```

تست‌های PG جدید: `TestPG_UpsertCustomerByPhoneIsIdempotent`،
`TestOTPVerify_DefaultsPurposeToLogin` — هر دو PASS.

### ۴.۲۷ بازرسی پوشش PG و بستن سه شکاف پرمخاطره — کامیت `test(pg-coverage)`

**درس دو باگ قبلی:** هر متد store که SQL اجرا می‌کند باید حداقل یک بار روی schema واقعی
اجرا شود؛ تست با store حافظه‌ای `42P10` و نوشتن خاموش را نمی‌بیند.

**ابزار تکرارپذیر:** `scripts/audit-pg-coverage.py` — فهرست متدهای SQL-دار بدون تست PG را
می‌دهد و با `--fail-under` می‌تواند در CI گیت شود.

```
قبل:  SQL methods=82  covered=48 (58.5%)  uncovered=34
بعد:  SQL methods=82  covered=60 (73.2%)  uncovered=22
```

**تست‌های تازه (همه PASS روی schema واقعی):**

- `TestPG_PaymentIntentAndAttemptReads` — intent فعال، `SuccessfulAttemptID`،
  `SumSucceededAttempts`، `UpdateOrderRefunded`
- `TestPG_ReservationAndBalanceWrites` — upsert موجودی (دو بار: باید update کند نه
  duplicate)، رزرو، خواندن، تغییر وضعیت
- `TestPG_ShipmentWrites` — محموله، تغییر وضعیت، رخداد، جمع تخصیص

**باگ چهارم از همین کلاس در حین تست:** `UpdatePaymentIntentStatus` هم `RowsAffected` را
بررسی نمی‌کرد → حالا `ErrPaymentIntentNotFound`.

**۲۲ متد باقی‌مانده** (همه خواندنی یا مسیرهای کم‌ریسک: `GetCart`, `GetTopic`,
`GetWarehouse`, `media.*`, `suppliers.ListSupplierVariantsByVariant`, …) در فهرست
`scripts/audit-pg-coverage.py` ثبت شده‌اند.

### ۴.۲۸ بستن پوشش PG تا ۱۰۰٪ و سخت‌کردن گیت — کامیت `test(pg-coverage-100)`

```
قبل:  81 متد SQL-دار · 60 پوشش (73.2%) · 21 بدون تست
بعد:  81 متد SQL-دار · 81 پوشش (100%)  ·  0
```

**تست‌های تازه:** `media` (اولین تست PG این ماژول)، انبار (`GetWarehouse`,
`GetWarehouseByCode`)، کاتالوگ (`ListProductsByCategory` — با اثبات اینکه پیش‌نویس در
فهرست منتشرشده نمی‌آید)، محتوا (`GetTopic`, `UpdateTopicStatus`, `SetArticleEditor`,
`UpdateArticleStatus`, `DeleteArticleSource`, `AddProductArticleRelation` با اثبات الزام
`relation_reason` در سطح DB)، کامرس (`GetCart`, `UpdateCartStatus`, `ListOrderItems`)،
تکمیل سفارش (`GetFulfillmentItem`, `UpdateFulfillmentItemStatus`,
`UpdateFulfillmentOrderStatus`, `ListFulfillmentOrdersByOrder`,
`ListShipmentItemsByShipment`)، هویت (`FindAdminByEmail` — شامل اثبات اینکه حساب بدون
`password_hash` هرگز برنمی‌گردد)، تأمین‌کننده (`ListSupplierVariantsByVariant`).

**ابزار:** اسکریپت بازرسی حالا helperهای غیرصادراتی را نادیده می‌گیرد (از طریق فراخوان
صادراتی پوشش داده می‌شوند) و **گیت CI از advisory به blocking** تغییر کرد
(`--fail-under 95`, `continue-on-error: false`).

**شاهد:** `python3 scripts/audit-pg-coverage.py --fail-under 95` → `GATE_PASS`

### ۴.۲۹ دروازهٔ a11y با axe در مرورگر واقعی — کامیت `test(a11y)`

`@axe-core/cli` در این محیط قابل نصب نیست: درایور chromedriver را از میزبان Google
می‌گیرد که **از این موقعیت جغرافیایی مسدود است** («not available in your location»)، و
Chrome هم نصب نیست. اما Lightpanda CDP را می‌فهمد و axe-core در آن اجرا می‌شود.

`scripts/a11y-audit.mjs` — بدون هیچ وابستگی (WebSocket داخلی Node 26):
اپ بیلدشده را سرو می‌کند، axe را با تگ `<script>` تزریق می‌کند (payload ۶۵۰KB از کانال
`Runtime.evaluate` رد نمی‌شود)، مجموعهٔ WCAG 2 A/AA را اجرا و در صورت تخلف با کد ۱ خارج
می‌شود.

**شاهد در هر دو جهت:**

```
main, shop, admin, magazine  → PASS (۱۵ قاعده) · «a11y: clean»
صفحهٔ عمداً معیوب            → FAIL: critical button-name، critical image-alt
```

**محدودیت ثبت‌شده:** مرحلهٔ نصب Lightpanda در GitHub runner هنوز تأیید نشده، پس job در CI
`continue-on-error: true` است تا یک بار سبز شود و بعد blocking شود.

### ۴.۳۰ تلاش برای OIDC در پنل و نتیجه‌ای که باید ثبت شود — بدون کامیت

پنل را به جریان PKCE وصل کردم (`adminLoginViaOIDC`: نشست → `authorize` → کد →
`/oauth/token`). **کار نکرد و دلیلش یک واقعیت معماری است، نه باگ:**

- `fetch(..., {redirect:'manual'})` هدر `Location` را در اختیار صفحه نمی‌گذارد (پاسخ
  opaque است) — پس کد از پاسخ خوانده نمی‌شود.
- راه استاندارد، ناوبری کامل صفحه به `/oauth/authorize` است؛ ولی آن درخواست نمی‌تواند
  هدر `Authorization` بفرستد.

یعنی **`/oauth/authorize` برای کلاینت مرورگری به نشست کوکی نیاز دارد** (یا باید
`continuation` را بدون Bearer بپذیرد). این یک تصمیم طراحی است و باید با مالک گرفته شود؛
تا آن زمان پنل روی ورود مستقیم کارمند (ایمیل + گذرواژه) می‌ماند که تست‌شده است.

کد آزمایشی برگردانده شد (بدون dead code) و ورود پنل دوباره تأیید شد: «سفارش‌ها» با ۴ سفارش.

### ۴.۳۱ نشست کوکی برای authorize + گارد CSRF — کامیت `feat(oidc-cookie)`

تصمیم مالک: `/oauth/authorize` با کوکی HttpOnly. پیاده شد:

- `SetSessionCookie` — HttpOnly، `SameSite=Lax`، `Secure` وقتی TLS، و **`Path=/oauth`** تا
  کوکی هرگز به `/api/v1/admin/*` نرود (کوکی لو رفته روی API کارمند قابل replay نیست).
- `bearerFromCookie` فقط برای **GET** مقدار برمی‌گرداند؛ هر متد دیگر (POST) نادیده گرفته
  می‌شود.
- گارد CSRF: authorize با کوکی **فقط** وقتی پذیرفته می‌شود که `code_challenge` و `state`
  هر دو حاضر باشند — کد بدون verifier بی‌ارزش است و `state` پاسخ را به درخواست خود کلاینت
  می‌بندد.
- ورود کارمند (`/api/v1/auth/admin/login`) همان نشست را در کوکی هم می‌گذارد.

**باگ حین کار:** `writeOIDCErr` برای `ErrOIDCInvalidRequest` به شاخهٔ `default` می‌افتاد و
**۵۰۰** می‌داد؛ هر درخواست بدشکل authorize (نبود `code_challenge`، `response_type` غلط)
۵۰۰ می‌گرفت نه ۴۰۰. اصلاح شد.

**شاهد (curl، با/بدون کوکی):**

```
cookie + PKCE + state → 302 (کد صادر شد)
cookie، بدون state    → 400   ← گارد CSRF
cookie، بدون PKCE     → 400
بدون کوکی            → 302 به صفحهٔ ورود
```

**باقی‌مانده:** اتصال پنل به این جریان (ناوبری کامل صفحه → `/auth/callback` → تبادل کد).
یک بار پیاده شد ولی در هارنس تست به حلقهٔ ریدایرکت خورد و برگردانده شد؛ پنل فعلاً روی
ورود مستقیم کارمند است. `scripts/dev/003_dev_oidc_redirect.sql` برای وایت‌لیست کردن
`http://127.0.0.1:5180/auth/callback` در محیط توسعه اضافه شد.

### ۴.۳۲ Storybook برای Design System — کامیت `feat(storybook)`

`web/.storybook` با فریم‌ورک `@storybook/react-vite` ساخته شد و **همهٔ استوری‌ها در
`fa-IR` / RTL** رندر می‌شوند — کامپوننتی که در LTR سالم به نظر می‌رسد ممکن است در محصول
واقعی بشکند.

استوری‌ها: Button (اصلی/ثانویه/غیرفعال/پیوند)، Badge (چهار tone)، Card، StateBlock
(loading/empty/error)، ProductCard، ArticleCard، و صفحه‌آرایی (Header/Footer/Container).

**دروازهٔ a11y حالا روی Storybook هم اجرا می‌شود:**

```
/iframe.html?id=design-system-components--buttons        PASS (۱۰ قاعده)
/iframe.html?id=design-system-components--states         PASS
/iframe.html?id=design-system-components--product-cards  PASS
```

اسکریپت‌های npm: `storybook` (dev)، `build-storybook`، `a11y`.

### ۴.۳۳ اتصال پنل به جریان کوکی — تلاش دوم، باز هم برگردانده شد

پیاده شد (ناوبری کامل صفحه → `/auth/callback` → تبادل کد با verifier). در مرورگر
**daemon قفل می‌کند** هر بار که اپ با `?code=` بوت می‌شود — یعنی نشانهٔ یک حلقه در مسیر
بوت کلاینت است، نه فقط هارنس. چون نتوانستم تأیید کنم، دوباره برگرداندم و پنل روی ورود
مستقیم کارمند ماند (کار و تست‌شده).

**آنچه تأیید شده:** سمت سرور کامل کار می‌کند (۴.۳۱): کوکی ست می‌شود، authorize با کوکی +
PKCE + state کد می‌دهد، و بدون state/PKCE با ۴۰۰ رد می‌شود.

**گام بعدی مشخص:** پروب روی `document.location.search` در اولین خط بوت (قبل از React)،
تا معلوم شود آیا `?code=` اصلاً به اپ می‌رسد یا هارنس آن را در fallback از دست می‌دهد.

**تلاش سوم و یافته‌های آن (همه در هارنس، نه محصول):**

1. fallback هارنس با کوئری سالم است: `GET /auth/callback?code=abc` → **۲۰۰** و همان HTML
   بدون ریدایرکت (با curl تأیید شد).
2. `history.replaceState` از مسیر بوت حذف شد (مرورگر مینیمال ممکن است روی آن قفل کند) —
   مشکل باقی ماند.
3. **باگ واقعی هارنس:** `do_POST` فقط `/api/` را پروکسی می‌کرد، پس `POST /oauth/token` با
   **۴۰۵** رد می‌شد و کلاینت پیام «fetch error» می‌گرفت. حالا `/oauth/` هم پروکسی می‌شود.
4. پس از رفع آن، daemon مرورگر در هر بوت با `?code=` قفل می‌کند — حتی با session تازه.

**نتیجه:** سمت سرور تأییدشده؛ مسیر بوت کلاینت هنوز تأیید نشده و کد برگردانده شد.

### ۴.۳۴ ریشه پیدا شد: پروکسی تست خودش ریدایرکت را دنبال می‌کرد — کامیت `feat(admin-oidc)`

با صفحهٔ پروب ایستا معلوم شد مرورگر با کوئری مشکلی ندارد (`/probe-cb.html?code=abc` →
`search=?code=abc&state=xyz`) و اپ هم در `/auth/callback?x=1` سالم بوت می‌شود. پس مشکل
نه مرورگر بود و نه اپ.

**ریشه:** هارنس تست با `urllib.request.urlopen` پروکسی می‌کرد و urllib **خودش ۳۰۲ را دنبال
می‌کند**. یعنی مرورگر هرگز ۳۰۲ را نمی‌دید، خودش ریدایرکت را انجام نمی‌داد و کد هرگز به
`/auth/callback` نمی‌رسید. با یک `HTTPRedirectHandler` که `redirect_request` را `None`
برمی‌گرداند، پروکسی حالا ۳۰۲ و هدر `Location` را به مرورگر پاس می‌دهد.

**شاهد — ورود کامل پنل با OIDC در مرورگر:**

```
href: http://127.0.0.1:5180/auth/callback?code=8de3aacd…
h1:   سفارش‌ها
orders: 4   (KHA-ORD-2026-000002 · PENDING_PAYMENT · ۹۰٬۰۰۰ تومان)
```

پنل حالا واقعاً از جریان Authorization Code + PKCE رد می‌شود: ورود کارمند → کوکی HttpOnly
→ authorize → کد → تبادل با verifier → access token → API.

**نکتهٔ باقی‌مانده:** کد در نوار آدرس می‌ماند (عمداً، چون `history.replaceState` در مرورگر
مینیمال قفل می‌کرد). کد یک‌بارمصرف است، ولی رفرش همان تب خطای «جریان منقضی» می‌دهد و کاربر
باید دوباره وارد شود.

### ۴.۳۵ سه شکاف تولیدی که همین جریان OIDC لو داد — کامیت `fix(deploy-oidc)`

جریان OIDC که در ۴.۳۴ کار کرد، سه چیز را در **تنظیمات تولید** لو داد:

1. **`/auth/callback` در nginx به سرویس هویت پروکسی می‌شد.** بلوک `location /auth/` وجود
   داشت و مسیر callback پنل را می‌بلعید → در تولید ۴۰۴، در حالی که در تست کار می‌کرد.
   حالا `/auth/callback` از SPA سرو می‌شود و فقط `/api/v1/auth/` به سرویس هویت می‌رود.
2. **`/oauth/` اصلاً پروکسی نبود.** `/oauth/authorize` به `try_files` می‌افتاد و HTML
   برمی‌گرداند — یعنی ریدایرکت هرگز اتفاق نمی‌افتاد. برای هر چهار دامنه اضافه شد.
3. **کوکی در تولید `Secure` نبود.** سرویس Go پشت nginx، `r.TLS` را nil می‌بیند؛ کوکی
   HttpOnly بدون `Secure` روی HTTP هم فرستاده می‌شد. `requestIsSecure` حالا
   `X-Forwarded-Proto` را هم می‌خواند و nginx آن هدر (و `Host`/`X-Real-IP`/
   `X-Forwarded-For`) را می‌فرستد.

تست‌های تازه: `TestSessionCookieFlags` (سه حالت: TLS مستقیم، پشت nginx، HTTP ساده) و
`TestBearerFromCookieIgnoresNonGET` (کوکی هرگز متد تغییردهنده را احراز نمی‌کند).

### ۴.۳۶ `operationId` برای هر ۵۸ عملیات — کامیت `docs(openapi)`

قرارداد هیچ `operationId` نداشت (۰ از ۵۸)، پس تولید کلاینت از روی spec نام‌گذاری تصادفی
می‌گرفت. برای همهٔ عملیات نام camelCase اضافه شد (`postAuthOtpRequest`,
`getOauthAuthorize`, `getAdminOrders`, …).

**شاهد:** `npx @stoplight/spectral-cli lint api/openapi.yaml --ruleset api/.spectral.yaml
--fail-severity=error` → **0 errors** (۲۹ warning از نوع `operation-description` که از قبل
بودند و گیت CI روی error است، نه warning).

### ۴.۳۷ بازرسی ریزبینانهٔ لایهٔ استقرار و قرارداد — دو ابزار گارد

**۱) سطح env — ناسازگاری واقعی پیدا شد.** `scripts/audit-env.py` (کد `config.Load` را با
`.env.example` مقایسه می‌کند):

```
قبل:  خوانده‌شده ۱۹ · مستند ۱۶
  مستندنشده: KH_AUTH_CODE_TTL_SECONDS، KH_OIDC_ISSUER،
             KH_WORKER_INTERVAL_SECONDS، KH_WORKER_BATCH_SIZE
  مرده:      KH_CARRIER_ADAPTER   ← هیچ کدی آن را نمی‌خواند
بعد:  ۱۹/۱۹ — «env surface is consistent»
```

`KH_CARRIER_ADAPTER` حذف شد (با توضیح در فایل) چون آداپتر حامل هنوز وجود ندارد؛ نگه‌داشتنش
اپراتور را گمراه می‌کرد. سه متغیر worker هم مستند شدند (فقط برای `KH_ROLE=worker`).

**۲) مسیرها در برابر قرارداد — شکاف بزرگ R-07 با عدد.**

```
scripts/audit-routes.py
  mounted: 90   declared: 58
  ۶۸ مسیر mount شده که در قرارداد نیست   (مثل GET /admin/products، /products/search)
  ۳۶ مسیر در قرارداد که mount نشده        (مثل /catalog/products، /magazine/articles، /cart)
```

این همان «اختلاف مسیر با R-07» است که قبلاً به‌صورت کیفی ثبت شده بود؛ حالا عدد دارد و
**قرارداد و کد دو شکل مختلف API را توصیف می‌کنند** (`/catalog/products` در برابر
`/products`). طبق قاعدهٔ پروژه، بازنویسی خاموش قرارداد ممنوع است — تصمیم با مالک است.

**CI:** job تازهٔ `config-audit` — گیت env **blocking** (`--fail-on-missing`)، گیت routes
فعلاً advisory تا تصمیم R-07.

**۳) فایل‌های env هر unit.** unitها به `/etc/khabavar/<unit>.env` اشاره می‌کردند ولی مخزن
فقط یک `.env.example` داشت — اپراتور باید حدس می‌زد هر unit چه می‌خواهد. سه فایل
`deploy/env/khabavar-{identity,api,worker}.env.example` اضافه شد و `audit-env.py` حالا
بررسی می‌کند هر نامی در آن‌ها واقعاً توسط `config.Load` خوانده می‌شود.

**باگ چهارم که همین کار لو داد:** worker هم `cfg.HTTPAddr` را سرو می‌کند و پیش‌فرضش
`:8080` است — یعنی با unit `api` روی همان پورت تصادم می‌کرد و یکی از دو سرویس بالا
نمی‌آمد. در فایل worker `KH_HTTP_ADDR=:8082` تنظیم شد.

### ۴.۳۸ گزینهٔ ب: کد از قرارداد تبعیت می‌کند — کامیت `feat(contract-aliases)`

تصمیم مالک: مسیرهای قرارداد باید کار کنند (نه اینکه قرارداد بازنویسی شود). aliasها با
**همان handler** اضافه شدند، نه پیاده‌سازی دوم:

```
GET  /catalog/categories            → CategoryTreeHandler
GET  /catalog/products              → SearchProductsHandler
GET  /catalog/products/{id}         → ProductDetailHandler
GET  /magazine/articles             → ListPublishedArticlesHandler
GET  /magazine/articles/{slug}      → GetArticleBySlugHandler
GET  /magazine/topics               → ListTopicsHandler
POST /auth/otp/request              → OTPStartHandler
POST /admin/articles/{id}/relations → AddProductRelationHandler (content.relation.manage)
POST /admin/orders/{id}/cancel      → CancelOrderHandler (order.manage)
```

**شاهد — برابری بایت‌به‌بایت هر جفت مسیر روی باینری واقعی:**

```
PASS catalog/categories    200:1666    PASS magazine/articles        200:793
PASS catalog/products      200:1313    PASS magazine/topics          200:1947
PASS catalog/products/{id} 200:501     PASS magazine/articles/{slug} 200:1202
```

aliasهای ادمین بدون توکن **۴۰۱** می‌دهند (mount شده‌اند و گارد RBAC جلویشان است، نه ۴۰۴)؛
`otp/request` هم مثل `otp/start` پاسخ **۲۰۲** می‌دهد.

**بازرسی مسیرها: ۳۶ → ۲۵.** ۲۵ مسیر باقی‌مانده **قابلیت پیاده‌نشده‌اند**، نه alias: سبد
خرید HTTP، `/me` مشتری، پرداخت (webhook/return)، return-requests، oauth logout،
PATCHهای پنل، shipment events، `GET /catalog/products/{id}/articles`.

### ۴.۳۹ سبد خرید سرورساید (۴ از ۶ مسیر قرارداد) — کامیت `feat(cart-api)`

قرارداد `GET /cart`, `POST /cart/items`, `PATCH|DELETE /cart/items/{id}` را می‌خواست و
هیچ‌کدام پیاده نشده بود. store سبد (`GetCart`, `UpdateCartStatus`) وجود داشت ولی
ساخت/افزودن/تغییر/حذف/ادغام نه.

**افزوده شد:** `CartStore` + `CartView` (با قیمت روز) + سرویس + چهار روت عمومی؛ مهمان با
هدر `X-Guest-Cart-Token` شناسایی می‌شود (طبق قرارداد).

**دو باگ که همین کار لو داد:**

1. **نام جدول قیمت اشتباه بود.** کوئری قیمت را از `price_records` می‌خواند؛ جدول واقعی
   `product_price_records` است. هر خواندن سبد با آیتم، ۵۰۰ می‌داد. (تست PG همین را گرفت،
   نه بازبینی چشمی.)
2. **ایندکس یکتای سبد جزئی است** (`uq_carts_active_session … WHERE status='ACTIVE'`) —
   همان درس باگ ورود مشتری: `ON CONFLICT` باید همان شرط را تکرار کند، وگرنه 42P10.

**قاعدهٔ قیمت:** قیمت هر قلم از **رکورد تأییدشدهٔ روز** خوانده می‌شود، نه از سبد. قلمی که
قیمت تأییدشده ندارد با `availability: UNAVAILABLE` و قیمت صفر برمی‌گردد تا فروشگاه
«قابل فروش نیست» بگوید، نه عدد کهنه.

**شاهد — جریان کامل روی باینری واقعی:**

```
بدون توکن        → 400
GET /cart        → 200 (سبد مهمان ساخته شد)
POST item x2     → 200 (۹۰۰٬۰۰۰ × ۲)
PATCH qty=3      → 200  line_total=2,700,000  subtotal=2,700,000
PATCH qty=0      → 200  items=0   (طبق قرارداد: صفر = حذف)
DELETE ناموجود   → 404
quantity=99      → 400
```

**باقی‌مانده از این خوشه:** `POST /cart/merge` (نیاز به هویت مشتری روی مسیر عمومی —
همان تصمیم احراز هویت) و `POST /checkout` (ایجاد سفارش از سبد + payment intent).

### ۴.۴۰ `POST /checkout` — سبد به سفارش + payment intent — کامیت `feat(checkout)`

ترتیب طبق DB §14.1: اعتبارسنجی سبد و قیمت روز → ایجاد Order با snapshot → تبدیل سبد به
CONVERTED → صدور Payment Intent. رزرو موجودی بعد از تأیید پرداخت انجام می‌شود (PRD O-4)،
نه اینجا.

**قاعده‌های پیاده‌شده:**
- قلم بدون قیمت تأییدشده → **۴۲۲** با فهرست اقلام متأثر (`checkout_unavailable`)، نه قیمت صفر.
- سبد خالی → **۴۰۹** `cart_empty`.
- مهمان بدون `guest_phone` → **۴۰۰** `guest_phone_required` (MA-8).
- سبد **بعد از** ایجاد سفارش تبدیل می‌شود، پس شکست سفارش سبد را سالم می‌گذارد.
- اگر کلاینت `items` بفرستد (بدون `cart_id`)، اول در سبد جمع می‌شوند تا فقط یک مسیر قیمت‌گذاری وجود داشته باشد.

**شاهد — روی باینری واقعی:**

```
سبد خالی        → 409 cart_empty
بدون guest_phone → 400 guest_phone_required   (قبلاً ۵۰۰ بود — رفع شد)
checkout        → 201  order KHA-ORD-2026-000004 · PENDING_PAYMENT · 1,800,000
                  payment intent ba9b266a… · amount 1,800,000 IRR
سبد بعد         → items: 0
checkout دوباره → 409 (سبد تبدیل‌شده، سبد تازه خالی است)
```

**باقی‌مانده:** `Idempotency-Key` روی checkout هنوز اعمال نمی‌شود (جدول idempotency وجود
دارد ولی برای این مسیر سیم نشده) و `redirect_url` درگاه (آداپتر Mock آدرس نمی‌دهد).

### ۴.۴۱ `Idempotency-Key` روی checkout — کامیت `feat(idempotency)`

قرارداد هدر `Idempotency-Key` را **required** می‌داند و ADR-ARCH-008 می‌گوید درخواست
تکراری باید همان نتیجه را برگرداند. تا امروز فقط `payment_attempts` ستون idempotency داشت،
نه خود درخواست — یعنی یک retry شبکه‌ای می‌توانست **دو سفارش** بسازد.

**افزوده شد:** مهاجرت `000015_idempotency` + `IdempotencyStore` + wrapper
`commerce.Idempotent`:

- بدون هدر → **۴۰۰** `idempotency_key_required`
- اولین درخواست کلید را claim می‌کند، اجرا می‌شود و پاسخش (status + body) ذخیره می‌شود
- retry با همان کلید و همان body → **همان پاسخ، بایت‌به‌بایت** (هدر `Idempotent-Replay: true`)
- همان کلید با body متفاوت → **۴۰۹** `idempotency_conflict`
- همان کلید در حال اجرا → **۴۰۹** `idempotency_in_flight` (نه سفارش دوم)
- فقط پاسخ‌های موفق ذخیره می‌شوند؛ خطا قابل retry می‌ماند

**هش canonical:** بدنه قبل از هش به JSON دوباره کد می‌شود، پس ترتیب کلید و فاصله‌ها
درخواست را «متفاوت» نشان نمی‌دهد (وگرنه یک retry بی‌ضرر ۴۰۹ می‌گرفت).

**شاهد — روی باینری واقعی:**

```
بدون کلید          → 400 idempotency_key_required
اولین درخواست      → 201  KHA-ORD-2026-000005
retry همان کلید    → 201  KHA-ORD-2026-000005   ← همان سفارش
همان کلید، body دیگر → 409 idempotency_conflict
تعداد سفارش با آن شماره در DB → 1   ← سفارش دومی ساخته نشد
```

### ۴.۴۲ سطح مشتری `/me` + دفترچهٔ آدرس — کامیت `feat(customer-me)`

قرارداد `GET /me`، `/me/addresses` (GET/POST/PATCH/DELETE) را می‌خواست و هیچ‌کدام نبود.
مسیرهای عمومی هیچ احراز هویتی نداشتند، پس اول یک mount تازه ساخته شد:

- `apiMux.customer` با `authorization.AuthenticateCustomer`: توکن باید به **مشتری** حل شود؛
  توکن کارمند صراحتاً رد می‌شود تا سطح مشتری درِ پشتی هویت کارمند نباشد (و برعکس).
- `CustomerStore` + `DBCustomerStore`: پروفایل، فهرست/ایجاد/ویرایش/حذف آدرس.

**قاعده‌های پیاده‌شده:** فقط یک آدرس پیش‌فرض (آدرس جدید پیش‌فرض، قبلی را در همان تراکنش
پاک می‌کند)؛ هر عملیات آدرس با `customer_id` محدود است، پس آیدی یک مشتری دیگر ۴۰۴ می‌دهد.

**شاهد — با ورود واقعی OTP و توکن مشتری:**

```
GET /me            → 200  (+989…0777، phone_verified: true)
POST address       → 201
GET addresses      → 200  count: 1  default: true
PATCH address      → 200
DELETE address     → 204
DELETE دوباره      → 404
توکن ادمین روی /me → 401   ← سطح مشتری درِ پشتی نیست
```

**بازرسی مسیرها: ۲۰ → ۱۵.** `/me/orders` (فهرست سفارش‌های مشتری) و `POST /cart/merge`
باقی مانده‌اند؛ merge حالا فقط به یک handler نیاز دارد چون هویت مشتری موجود است.

### ۴.۴۳ ادغام سبد مهمان + سفارش‌های مشتری — کامیت `feat(cart-merge)`

دو مسیر باقی‌ماندهٔ خوشهٔ مشتری:

- **`POST /cart/merge`** (MA-8): سبد مهمان به سبد مشتری ادغام می‌شود. `FindCartBySession`
  عمداً **نمی‌سازد**: توکنی که سبد ندارد no-op است، نه یک سبد خالی جدید. ادغام دوباره
  تکرار نمی‌کند (سبد مهمان CONVERTED می‌شود) — تست همین را ثابت می‌کند.
- **`GET /me/orders`**: فهرست سفارش‌های خود مشتری، با `customer_id` در SQL محدود شده؛
  تست ثابت می‌کند مشتری دیگر صفر سفارش می‌بیند.

**بازرسی مسیرها: ۱۵ → ۱۳.**

**شاهد (PG، روی schema واقعی):** `TestPG_MergeGuestCartAndCustomerOrders` — ادغام،
بی‌اثر بودن ادغام دوم، `FindCartBySession` برای توکن ناشناخته `nil` (بدون ساخت سبد)، و
جداسازی سفارش‌های دو مشتری.

### ۴.۴۴ webhook درگاه + بازگشت مرورگر — کامیت `feat(payments-webhook)`

بدون این دو، هیچ سفارشی از `PENDING_PAYMENT` خارج نمی‌شد.

**`POST /payments/webhook`:**
- امضای `X-Provider-Signature` روی **بدنهٔ خام** با HMAC-SHA256 بررسی می‌شود (کدگذاری
  دوبارهٔ JSON امضا را می‌شکند). کلید خالی → **fail closed**، نه fail open.
- اعلان خام هرگز مبنای تأیید نیست؛ فقط می‌گوید کدام تراکنش را نگاه کنیم. تأیید از مسیر
  `ConfirmPaymentForOrder` می‌رود که payment attempt و رویداد outbox را در یک تراکنش می‌نویسد.
- مرجع ناشناخته → **۲۰۲** (درگاه دست از retry بردارد، ولی چیزی تأیید نشده).
- **retry درگاه → ۲۰۰ `already_confirmed`**؛ اگر ۵۰۰ می‌دادیم درگاه تا ابد تکرار می‌کرد.

**`GET /payments/return`:** فقط مرورگر را به صفحهٔ سفارش هدایت می‌کند (۳۰۲) و خودش چیزی
تأیید نمی‌کند. مقصد از config می‌آید تا open redirect نشود.

**دو باگ حین کار:**
1. `ConfirmPayment(intentID)` یک stub است که همیشه خطا می‌دهد؛ مسیر واقعی
   `ConfirmPaymentForOrder` است. webhook اول ۵۰۰ می‌داد.
2. retry درگاه ۵۰۰ می‌گرفت (سفارش قبلاً PAID بود) — حالا ۲۰۰.

**شاهد — روی باینری واقعی:**

```
بدون امضا      → 401
امضای غلط      → 401
امضای درست     → 200 {"status":"confirmed"}   → وضعیت سفارش: PAID
retry همان اعلان → 200 (already_confirmed)
مرجع ناشناخته  → 202
/payments/return → 302 https://shop.khabavar.ir/order/KHA-ORD-2026-000008
```

**بازرسی مسیرها: ۱۳ → ۱۱.**

### ۴.۴۵ محموله و رویدادهای رهگیری — کامیت `feat(shipments)`

دو مسیر قرارداد: `POST /admin/fulfillments/{id}/shipments` و
`POST /admin/shipments/{id}/events`.

- `CreateShipmentHandler` از قبل نوشته شده بود ولی **mount نشده بود** — فقط وصل شد.
- `Service.AddShipmentEvent` تازه: رخداد رهگیری را به تایم‌لاین اضافه می‌کند **بدون**
  تغییر وضعیت محموله. تغییر وضعیت باید از `TransitionShipment` بگذرد تا ماشین حالت
  سالم بماند.
- منبع رخداد قبل از هر نوشتن اعتبارسنجی می‌شود (CHECK فقط MANUAL/CARRIER_API/WEBHOOK را
  می‌پذیرد)؛ رخداد بی‌کد رد می‌شود.

**شاهد (PG):** `TestPG_AddShipmentEvent` — رخداد ثبت می‌شود، وضعیت محموله دست‌نخورده
می‌ماند، و منبع نامعتبر پیش از درج رد می‌شود (تعداد رخدادها تغییر نمی‌کند).

**بازرسی مسیرها: ۱۱ → ۹.**

### ۴.۴۶ ویرایش مقاله + بازبینی در پنل — کامیت `feat(article-edit)`

- **`PATCH /admin/articles/{id}`**: `Service.UpdateArticle` + `DBStore.UpdateArticle`.
  فیلدها pointer-based‌اند (PATCH نباید فیلدی را که ذکر نشده خالی کند)، **وضعیت قابل
  ویرایش نیست** (گردش کار مسیرهای محافظت‌شدهٔ خودش را دارد)، و slug/body خالی رد می‌شود.
- **`POST /admin/articles/{id}/review`**: قرارداد یک endpoint بازبینی می‌خواست؛ به‌جای
  ماشین حالت دوم، `decision` روی همان انتقال‌های محافظت‌شده نگاشت می‌شود
  (`submit`/`approve`/`reject`). ویرایشگرِ تأییدکننده از **actor احرازشده** می‌آید، نه از بدنه.

**شاهد (PG):** `TestPG_UpdateArticleAndReview` — ویرایش عنوان با slug دست‌نخورده، وضعیت
بعد از PATCH همان DRAFT، رد شدن slug/body خالی، رد شدن submit دوباره روی IN_REVIEW، و
چرخهٔ submit → approve → reject.

**بازرسی مسیرها: ۹ → ۷.**

### ۴.۴۷ مقالات مرتبط با محصول — کامیت `feat(product-articles)`

`GET /catalog/products/{id}/articles`: خواندن مرتبط با محصول در صفحهٔ محصول.

- `DBStore.ListPublishedArticlesForProduct` — فیلتر `status = 'PUBLISHED'` **در SQL** است،
  نه در فراخواننده: ویترین هرگز نباید از طریق رابطهٔ محصول به یک DRAFT برسد.
- `ListRelationsForProduct` از قبل بود؛ این جهتِ معکوس (رابطه → مقاله) را اضافه می‌کند.

**شاهد (PG):** `TestPG_PublishedArticlesForProduct` — یک مقالهٔ منتشرشده و یک پیش‌نویس،
هر دو مرتبط با همان محصول؛ خروجی فقط منتشرشده را برمی‌گرداند.

**بازرسی مسیرها: ۷ → ۶** (بقیه: `PATCH /admin/suppliers`، `PATCH /admin/users`،
`POST /admin/users`، `POST /admin/orders/{}/fulfillments`، `POST /oauth/logout`،
`POST /orders/{}/return-requests`).

---

## ۵. کارهای باقی‌مانده (به ترتیب اولویت)

1. **روت‌های ادمین** — کاتالوگ، قیمت‌گذاری (§۴.۱۰/§۴.۱۱) و پنل تأمین (§۴.۱۲) بسته شدند.
   سطح HTTP قرارداد اکنون کامل است؛ اختلاف باقی‌مانده فقط در **مسیرها** است
   (`/api/v1/products/{id}` در برابر `/catalog/products/{slug}`) که تصمیم مالک می‌خواهد.
2. **فرانت‌اند** — Design System (`web/shared`)، اپ `main` (§۴.۱۷) و اپ `shop` با مسیر
   خرید کامل (§۴.۱۹) ساخته شدند. باقی: `magazine` و `admin` (هنوز scaffold)، و
   Storybook + دروازهٔ axe برای a11y.
3. **پوشش تست** — آستانهٔ R-CI §۳: ≥۹۰٪ مالی/موجودی، ≥۷۵٪ سایر. پس از تست‌های
   انتگرال §۴.۱۵: pricing ۶۱.۷٪، suppliers ۶۰.۱٪، catalog ۶۲٪، fulfillment ۵۲.۲٪،
   inventory ۴۴.۳٪. وضعیت پیش از آن:
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
