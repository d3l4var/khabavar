KHABAVAR — CI Pipeline & Quality Gates Specification v1.1
Document ID: KH-CI-001
Version: 1.1 — Draft for Owner Review (بازبینی 2026-11-25 — هم‌سویی با قرارداد v1.1.0 و ماتریس ۱۰نقشه)
Date: 2026-11-25
مبنای نسخه ۱: Test Strategy KH-TS-001 (R-17)، Deployment Spec KH-DEP-001 (R-18)، SAD v2.0 KH-SAD-001 (R-16)، OpenAPI KH-API-001 (R-07)، DB Migrations (R-14)، IAM Matrix KH-IAM-MATRIX-001 (R-11)
هدف این نسخه: تعریف خط لوله یکپارچه CI/CD که هر تغییری پیش از ادغام در main/staging از دروازه‌های کیفیت مصوب عبور کند؛ مهاجرت‌ها، قرارداد API، پوشش آزمون، امنیت، دسترس‌پذیری و عملکرد به‌صورت قابل‌تکرار و خودکار راستی‌آزمایی شوند؛ و انتشار با برگشت‌پذیری اتمیک انجام گیرد. سند اجرایی هم‌راه: download/khabavar-ci/.github/workflows/ci.yml.

۱. خط‌مشی دروازه‌بندی
* هفت دروازه باید برای ادغام در main بگذرند: lint، openapi-validate، db-migrate، unit، integration، security، a11y. شکست هر یک مانع ادغام می‌شود (branch protection با required status checks).
* دو دروازه شبانه روی staging اجرا می‌شوند: e2e (۱۲ جریان UJ) و perf (Lighthouse + k6). این دو مانع ادغام نیستند بلکه سیگنال کیفیت و شرط Go-Live طبق TS §5 هستند.
* انتشار staging روی push به staging و انتشار production روی push به main با محیط guarded و تأیید دستی انجام می‌شود (DEP §1). مهاجرت‌ها فقط forward هستند؛ down فقط از طریق workflow_dispatch با ورودی emergency_rollback_migration و در محیط guarded با تأیید اپراتور اجرا می‌شود (DEP §6).

۲. معماری خط لوله (۱۱ کار)
خط لوله از یازده کار تشکیل شده است که وابستگی‌های آن‌ها به‌گونه‌ای طراحی شده که سریع‌ترین بازخورد اول بیاید (lint و openapi-validate موازی) و پرهزینه‌ترین (e2e و perf) فقط شبانه روی staging اجرا شوند. کار build تنها پس از عبور چهار دروازه منطقی (unit/integration/security/a11y) اجرا می‌شود تا artifact آلوده ساخته نشود و انتشار همیشه روی خروجی تأییدشده باشد.
Table: کار | وابسته به | ماشه | هدف
lint | — | هر push/PR | go vet + golangci-lint + eslint + tsc + RTL/dict
openapi-validate | — | هر push/PR | Spectral OAS3.1 + قواعد سفارشی + PRISM mock smoke
db-migrate | — | هر push/PR | اعمال ۱۰ مهاجرت + seed روی PG 16 + آزمون نقض هر CHECK
unit | lint, openapi-validate, db-migrate | هر push/PR | go test -race + پوشش ≥۹۰٪ مالی/≥۷۵٪ سایر + vitest
integration | unit | هر push/PR | ۶ سناریوی هم‌روندی TS §3 + OTP abuse
security | lint | هر push/PR | gosec + npm audit + OWASP ZAP + ماتریس ۱۰نقش
a11y | lint | هر push/PR | axe WCAG AA روی storybook + ۳۶۰px بدون اسکرول افقی
build | unit, integration, security, a11y | هر push/PR | باینری Go تک‌باینری + bundle وب + smoke startup
e2e | build | شبانه/dispatch | ۱۲ جریان UJ روی staging (Playwright)
perf | build | شبانه/dispatch | Lighthouse + k6 checkout p95
deploy | build | push به staging/main | انتشار اتمیک symlink + smoke health

۳. دروازه‌های کیفیت و آستانه‌ها
Table: دروازه | ابزار | آستانه | سند مبنا
پوشش منطق مالی/موجودی | go cover | ≥۹۰٪ خط | TS §2
پوشش سایر بسته‌ها | go cover | ≥۷۵٪ خط | TS §2
پرداخت تکراری | آزمون idempotency ۳حالته | نرخ خطا = ۰ | PRD BR-10 / ADR-ARCH-008
رزرو منفی | آزمون نزدیک‌صفر | صفر رزرو منفی | TS §3 / DB §8.2
یک‌پرداخت موفق | uq_intent_success_per_order | یکتا حفظ شود | DB §10.5
نقض CHECK | هر CHECK یک تست نقض‌کننده | همه سبز | TS §1
کنتراست رنگ | axe + tokens.json | WCAG 2.1 AA | DS §2
نمای ۳۶۰px | check-viewport | بدون اسکرول افقی | DS §5 / O-10
LCP | Lighthouse | ≤۲۵۰۰ms | TS §4 / O-10
INP | Lighthouse | ≤۲۰۰ms | TS §4
checkout p95 | k6 @۵۰ کاربر | ≤۸۰۰ms | TS §4
جستجوی کاتالوگ p95 | k6 @۵۰ کاربر | ≤۵۰۰ms | TS §4
RBAC | آزمون ماتریس ۱۰نقش | همه ردیف‌ها سبز | IAM §4
امنیت | gosec medium+ / npm high+ / ZAP | صفر خطای high | TS §4

۴. مدیریت مهاجرت و داده
* هر push: سرویس postgres:16-alpine در container بالا می‌آید، مهاجرت‌های ۰۰۰۰۰۱ تا ۰۰۰۰۱۰ با golang-migrate اعمال و سپس seed 001 اعمال می‌شود؛ سپس راستی‌آزمایی می‌شود که ۱۰ نقش، ۲۴ مجوز و ۴ کلاینت OIDC درست نشسته‌اند (IAM §2/§6).
* هر قید CHECK در مهاجرت‌ها باید یک آزمون نقض‌کننده داشته باشد (TS §1): chk_products_publish_gates، chk_orders_totals، chk_items_line_total، uq_intent_success_per_order، payment_attempts.idempotency_key، chk_reservation_source، chk_fulfillment_source، و غیرمنفی بودن stock_balances.
* مهاجرت down فقط با ورودی emergency_rollback_migration در محیط guarded اجرا می‌شود و لاگ آن به Audit Log حساس می‌رود (DEP §6). ترتیب مهاجرت‌ها طبق DB §16 است؛ FKهای معوق (مثل inventory_reservations.order_item_id که در ۰۰۰۰۰۴ تعریف و در ۰۰۰۰۰۶ پر می‌شود) در آزمون ادغانی راستی‌آزمایی می‌شوند.

۵. قرارداد API و Idempotency
* فایل api/openapi.yaml با Spectral و قواعد سفارشی api/.spectral.yaml راستی‌آزمایی می‌شود: قاعده idempotency (هدر Idempotency-Key روی همه POSTهای مالی — ADR-ARCH-008)، قاعده Money (int64 IRR — ADR-DATA-001)، قاعده تفکیک scope (staff.* فقط برای کلاینت staff — IAM §6)، و شفافیت بسته (CheckoutResult شامل fulfillment_groups با parcel_count + eta — PRD MA-4).
* PRISM در حالت mock بالا می‌آید و یک smoke قرارداد روی همه مسیرهای تعریف‌شده اجرا می‌شود تا شکاف بین schema و پیاده‌سازی زود دیده شود و قرارداد پیش از نوشتن کلاینت قابل‌اجرایی بماند.

۶. محیط‌ها و OIDC
Table: محیط | ماشه | داده | دروازه انتشار
Development | هر PR | seed ساختگی | none (محلی)
Staging | push به staging | کپی ماسک‌شده | build سبز + smoke
Production | push به main | واقعی | build سبز + تأیید دستی guarded + smoke
* انتشار با OIDC انجام می‌شود (id-token: write، ADR-IAM-004) — بدون کلید SSH長دیمان. deploy host با کلید deploy اختصاصی (least privilege). انتشار با atomic symlink (ln -sfn) و reload انجام می‌شود؛ برگشت فوری با اشاره symlink به release قبلی است (DEP §5).
* نکته: DEP v1 §3 سه واحد systemd فهرست کرده بود و این با تک‌باینری SAD v2 §4 (ADR-DB-002) در تضاد بود — با تصمیم مالک 2026-11-25 رفع شد: هر سه unit از همان باینری واحد با متغیر نقش KH_ROLE اجرا می‌شوند (DEP v1.1 §۳).

۷. artifact و انتشار اتمیک
* artifact khabavar-dist شامل باینری Go و bundle وب است و ۳۰ روز نگه داشته می‌شود. coverage و گزارش e2e هم به‌عنوان artifact آپلود می‌شوند (retention ۱۴ تا ۳۰ روز).
* انتشار: artifact به release جدید کپی می‌شود، symlink current به release جدید اشاره می‌کند، سپس systemctl reload-or-restart اجرا می‌شود. برگشت: symlink به release قبلی برمی‌گردد — عملیاتی فوری و بدون نیاز به redeploy.
* post-deploy smoke: /healthz باید ok برگرداند و /api/v1/magazine/topics باید topics برگرداند تا تطبیق زنده قرارداد پس از هر انتشار راستی‌آزمایی شود.

۸. شکست و رفتار
* concurrency: cancel-in-progress=false تا هیچ مهاجرت یا انتشاری نیمه‌تمام لغو نشود — خط لوله تا پایان می‌دود.
* شکست دروازه‌های seven-gate: PR نمی‌تواند merge شود. شکست e2e/perf: سیگنال به تیم اما مسدودکننده نیست (جز در هفته Go-Live طبق TS §5 که همه دروازه‌ها باید سبز باشند).
* شکست در محیط guarded production: انتشار متوقف می‌شود و smoke برگشت می‌زند؛ اپراتور باید release قبلی را با symlink بازیابی کند.

۹. سجل تطبیق (تصمیمات مالک 2026-11-25 — اجرا در قرارداد v1.1.0)
* هشت مغایرت اولویت‌بالا با تصمیم مالک حل شد و سجل کامل آن در KH-INDEX-001 v1.1 §۷ ثبت است: تغییر شناسه ماتریس به KH-IAM-MATRIX-001، بازگشت نقش SUPPLIER_OPERATOR (Seed اکنون ۱۰ نقش دارد)، MIXED به‌عنوان استراتژی سطح محصول با حفظ enum دومقداری، چرخه عمر تأمین‌کننده با هشت وضعیت مصوب به‌علاوه PROBATION، افزودن POST /orders/{orderNumber}/return-requests، شش وضعیت مشتری‌روی موجودی، تک‌باینری با متغیر نقش در DEP v1.1، و ارجاع اکتورهای SAD §۳ به ماتریس.
* قواعد Spectral معلق‌شده (severity: off) فعال شدند و قاعده Idempotency-Key برای دقت بیشتر به چک regex مسیر با @property منتقل شد. دروازه‌های CI اکنون روی قرارداد v1.1.0 می‌ایستند.

۱۰. نقشه پوشش به اسناد
Table: سند | بخش مرتبط با CI | نحوه استفاده
TS v1 (R-17) | §1 هر CHECK یک تست نقض | کار db-migrate + unit
TS v1 | §2 آستانه‌های پوشش | کار unit coverage-gate
TS v1 | §3 ۶ سناریوی هم‌روندی | کار integration
TS v1 | §4 معیارهای عملکرد عددی | کار perf (Lighthouse/k6)
TS v1 | §5 UAT/Go-Live | کار e2e + شرط انتشار
DEP v1 (R-18) | §1 محیط‌ها | محیط‌های staging/prod
DEP v1 | §5 انتشار اتمیک symlink | کار deploy
DEP v1 | §۶ مهاجرت forward-only | خط‌مشی down فقط اضطراری
DEP v1 | §۷ نقش‌های DB | راستی‌آزمایی پس از deploy
SAD v2 (R-16) | ADR-ARCH-008 idempotency | قاعده Spectral + آزمون integration
SAD v2 | ADR-DB-002 تک‌باینری | کار build
OpenAPI (R-07) | کل قرارداد | کار openapi-validate + PRISM
IAM (R-11) | §2-§۴ نقش‌ها و مجوزها | راستی‌آزمایی seed + آزمون RBAC
IAM | §۶ کلاینت‌های OIDC | قاعده scope separation
DB Migrations (R-14) | قیدهای CHECK | کار db-migrate
