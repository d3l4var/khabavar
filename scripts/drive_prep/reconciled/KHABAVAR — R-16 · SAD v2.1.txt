KHABAVAR — Software Architecture Document (SAD) v2.1
Document ID: KH-SAD-001
Version: 2.1 — Approved (بازبینی 2026-11-25: §۳ اکتورها با IAM Matrix هم‌سو شد)
Date: 2026-11-24
Status: نسخه ۲.۱ — مصوب مالک (تأیید: 2026-11-24؛ بازبینی: 2026-11-25) | ۱۵ ADR پیشنهادی به ACCEPTED ارتقا یافت | ADR-DEP-001 و زیردامنه admin.khabavar.ir مصوب | §۳ اکتورها به ۱۰ نقش مصوب KH-IAM-MATRIX-001 ارجاع شد (سجل تطبیق KH-INDEX-001)
مبنای نسخه ۲: SAD v1.0 (تکمیل بخش‌های «نیازمند تکمیل» طبق Master §۵)، PRD v1.0 مصوب، Database Architecture v2.0، CIAM v1.0، Commerce Spec v1.0، Supplier Eval v1.0، Product Catalog v1.0
هدف این نسخه: تبدیل معماری مفهومی به معماری اجرایی — مرزهای ماژول، ADR Registry کامل، جریان‌های کلیدی، استانداردهای داده و استقرار. موارد نیازمند تصمیم مالک با ⚠️ مشخص شده‌اند.

۱. چشم‌انداز، دامنه و محدودیت‌های معماری

* دامنه: اکوسیستم خواب‌آور = Main Website + Shop + Magazine + Central Identity + Admin Panel (بازار ایران، fa-IR، RTL).
* مدل تجاری: فروشگاه تخصصی مدل A (تعریف قابل آزمون در PRD §۲ — مرجع معماری برای MA-1 تا MA-9).
* محدودیت‌های معماری (از Master §۴): اجتناب از Microservices غیرضروری، زیرساخت پیچیده و وابستگی‌های پرهزینه در نسخه اول؛ بدون الزام Docker؛ عملکرد در مقیاس اقتصادی (<۱۰۰۰ بازدید روزانه، قابل توسعه).
* محدودیت‌های اجرایی: تا تأیید مالک: بدون خرید سرور، درگاه واقعی، قرارداد تأمین‌کننده، سرویس پولی. همه یکپارچه‌های خارجی با Mock (Roadmap §۲).

۲. اصول معماری و الزام فنی هر اصل

اصل	الزام فنی قابل راستی‌آزمایی

Sleep First	دامنه محصول SLEEP در سطح کاتالوگ و QC اعمال می‌شود (MA-1)
Content & Commerce	ماژول Content و Commerce مستقل اما با رابطه چندبه‌چند (product_article_relations)
Trust First	فیلدهای منبع مشخصات/تاریخ تأیید اجباری؛ Audit کامل؛ سیاست سلامت محتوا
Unified Experience	هویت مرکزی + Design System مشترک + نشست مستقل هر سرویس
Modular Architecture	مرزهای ماژول §۴ با قواعد وابستگی الزامی
Operational Simplicity	یک باینری Go + یک PostgreSQL + فایل‌ها؛ بدون Kafka/K8s/ORMهای سنگین
Data Driven	Economics Register، financial snapshots، داشبوردها

۳. نمای سیستم (System Context)

اکتورها: مهمان | مشتری | مدیران — ۱۰ نقش کارمندی مصوب طبق IAM Matrix §۲ (KH-IAM-MATRIX-001؛ ارجاع به ماتریس به‌جای تکرار فهرست نقش‌ها تا drift نشود — تصمیم مالک 2026-11-25) | سیستم‌های خارجی (درگاه پرداخت، سرویس پیامک، شرکت حمل، Object Storage — همگی Mock در نسخه اول).

سرویس‌ها (استقرار):
* `khabavar.ir` — وب‌سایت اصلی (SSR/prerender)
* `shop.khabavar.ir` — فروشگاه (SPA + prerender صفحات کلیدی)
* `magazine.khabavar.ir` — مجله
* `auth.khabavar.ir` — صفحه احراز هویت مرکزی (جریان OIDC)
* `admin.khabavar.ir` (پیشنهاد نام؛ زیردامنه مصوب) — پنل مدیریت
* `api` — یک Backend واحد (Go) که همه سرویس‌های بالا را خدمت می‌دهد؛ زیردامنه‌ها مسیرهای مجاز با Scope را صدا می‌زنند.

۴. سبک معماری و مرز ماژول‌های Backend

سبک: Modular Monolith with Independent Deployable Frontends (SAD v1 §۱۰.۱) — یک باینری Go، ماژول‌های داخلی با مرز صریح، فرونت‌اند‌های مستقل قابل استقرار جدا.

۴.۱. ماژول‌ها

ماژول	مسئولیت	موجودیت‌های اصلی	وابستگی مجاز

identity	ثبت‌نام/ورود OTP، نشست‌ها، OIDC clients، چالش‌های OTP	users, user_credentials, user_sessions, identity_clients, otp_challenges	shared
authorization	نقش‌ها، مجوزها، Scopeهای سرویس‌محور	roles, permissions, user_roles, role_permissions	identity
catalog	محصول، Variant، دسته‌بندی، ویژگی‌ها، برند	products, product_variants, categories, attribute_*, product_media	media, shared
media	دارایی‌های رسانه‌ای + Adapter Object Storage	media_assets	shared
suppliers	تأمین‌کنندگان، ارزیابی وزنی، قیمت/موجودی تأمین با تاریخ اعتبار	suppliers, supplier_products, supplier_variants, supplier_evaluations	catalog, shared
inventory	انبار، موجودی، Ledger حرکت، رزرو تراکنشی	warehouses, stock_balances, stock_movements, inventory_reservations	catalog, shared
pricing	هزینه تمام‌شده، قیمت فروش، سود مشارکت، Snapshot مالی	product_cost_records, product_price_records, order_financial_snapshots	catalog, suppliers, shared
commerce	سبد، سفارش، اقلام، پرداخت (Intent/Attempt)، بازپرداخت	carts, cart_items, orders, order_items, payment_intents, payment_attempts, refunds	catalog, inventory, pricing, identity
fulfillment	تفکیک Fulfillment، مرسوله‌ها، رویدادهای حمل	fulfillment_orders, fulfillment_items, shipments, shipment_items, shipment_events	commerce, inventory, suppliers
content	مقالات، دسته‌ها، منابع، ترجمه، بازبینی تحریریه، رابطه مقاله-محصول	articles, topics, article_sources, editorial_reviews, product_article_relations	catalog (فقط شناسه)، identity
notifications	اعلان‌ها (نسخه اول: درون‌سیستمی + Mock پیامک)	notifications	shared
audit	Audit Log + Domain Events + Transactional Outbox	audit_logs, domain_events	همه فقط می‌نویسند
shared	پیکربندی، لاگ، health، محدودیت‌های نرخ	settings	—

۴.۲. قواعد وابستگی (الزامی)
1. هیچ ماژولی جدول ماژول دیگر را مستقیم UPDATE/INSERT نمی‌کند؛ مالکیت داده با ماژول است.
2. ارتباط بین‌ماژولی از طریق API داخلی (فراخوانی متد در مرز ماژول) + رویدادهای Outbox برای اثرات جانبی.
3. وابستگی دایره‌ای ممنوع؛ جهت مجاز: fulfillment→commerce→{catalog,inventory,pricing}؛ content→catalog (فقط id).
4. رویدادهای دامنه Commerce §۱۰ به‌صورت domain_events منتشر می‌شوند؛ مصرف‌کننده‌ها idempotent هستند.
5. هر عملیات حساس: Idempotency Key + Audit (ADR-ARCH-008/010).

۵. ADR Registry (مجموعه کامل)

شناسه	تصمیم	وضعیت فعلی اسناد	پیشنهاد این نسخه

ADR-CAT-001	دامنه محصولات SLEEP	ACCEPTED	حفظ
ADR-SUP-001	تأمین ترکیبی MIXED	ACCEPTED	حفظ
ADR-PRD-001	استراتژی Full Launch	ACCEPTED (Master §۳.۳)	حفظ
ADR-PRD-002	مدل A (۹ قاعده PRD §۲)	ACCEPTED	حفظ؛ مرجع پیاده‌سازی: MA-1…MA-9
ADR-COM-001	یک سفارش/یک پرداخت/چند Fulfillment و مرسوله	ACCEPTED	حفظ
ADR-COM-002	قیمت‌گذاری بر مبنای سود مشارکت	ACCEPTED	حفظ
ADR-DB-001	PostgreSQL تنها DB عملیاتی	ACCEPTED	حفظ
ADR-DB-002	Modular Monolith	PROPOSED	ACCEPTED ✓ (مصوب مالک)
ADR-DB-003	شناسه‌ها UUID + شناسه تجاری جدا (KHA-ORD-…)	PROPOSED	ACCEPTED ✓ (مصوب مالک)
ADR-BE-001	Backend با Go	PROPOSED (Master §۴)	ACCEPTED ✓ (مصوب مالک)
ADR-FE-001	Frontend React + TypeScript + Vite	PROPOSED	ACCEPTED ✓ (مصوب مالک)
ADR-API-001	REST + مستندسازی OpenAPI	PROPOSED	ACCEPTED ✓ (مصوب مالک)
ADR-DATA-001	واحد مالی IRR (BIGINT)؛ تومان فقط لایه نمایش	ACCEPTED (DB v2)	حفظ
ADR-DATA-002	زمان TIMESTAMPTZ/UTC؛ نمایش Asia/Tehran	ACCEPTED (DB v2)	حفظ
ADR-DATA-003	Snapshot قیمت/مشخصات/آدرس در سفارش	ACCEPTED (قواعد حیاتی)	حفظ
ADR-DATA-004	وضعیت‌ها TEXT + CHECK (نه ENUM زودهنگام)	پیشنهاد DB v2	ACCEPTED ✓ (مصوب مالک)
ADR-ARCH-007	Transactional Outbox برای رویدادها	پیشنهاد Commerce §۱۰	ACCEPTED ✓ (مصوب مالک)
ADR-ARCH-008	Idempotency پرداخت/تأیید/رزرو/بازپرداخت	الزام پذیرش Commerce §۱۴	ACCEPTED ✓ (مصوب مالک)
ADR-ARCH-009	پرداخت با الگوی Adapter + Intent/Attempt	پیشنهاد SAD §۶.۵ + DB v2 §۱۰.۵	ACCEPTED ✓ (مصوب مالک)
ADR-ARCH-010	Audit عملیات حساس مالی/قیمت/دسترسی	الزام Commerce §۱۱	ACCEPTED ✓ (مصوب مالک)
ADR-ARCH-011	Mock-first برای همه یکپارچه‌های خارجی	ACCEPTED (Roadmap §۲)	حفظ
ADR-IAM-001	ورود OTP تلفن — روش اصلی نسخه اول	ثبت‌شده CIAM §۴	ACCEPTED ✓ (مصوب مالک)
ADR-IAM-002	جریان OIDC (Auth Code + PKCE)	پیشنهاد CIAM §۳	ACCEPTED ✓ (مصوب مالک)
ADR-IAM-003	RBAC + Scopeهای سرویس‌محور	طراحی CIAM §۶	ACCEPTED ✓ (مصوب مالک)
ADR-IAM-004	نشست هر سرویس مستقل؛ بدون اشتراک Cookie به‌صورت پیش‌فرض	ثبت‌شده CIAM §۵	ACCEPTED ✓ (مصوب مالک)
ADR-DEP-001	استقرار Linux + systemd؛ بدون الزام Docker	تصمیم باز Master §۴	ACCEPTED ✓ (systemd، بدون Docker)
ADR-FE-002	RTL Native + Mobile First + Design Tokens	ACCEPTED (Brand §۷-۹)	حفظ
ADR-CONTENT-001	سیاست سلامت محتوا (بدون منبع/ادعای درمانی → انتشار ممنوع)	الزام Catalog/SAD	ACCEPTED ✓ (مصوب مالک)
ADR-SEARCH-001	جست‌وجو نسخه اول: PostgreSQL FTS + نرمال‌سازی ی/ک و نیم‌فاصله	پیشنهاد DB v2 §۱۵	ACCEPTED ✓ (مصوب مالک)

تصمیم مرحله ۲ (مصوب): همه ADRهای پیشنهادی با تأیید مالک (2026-11-24) به ACCEPTED ارتقا یافتند؛ ADR-DEP-001 (استقرار systemd بدون Docker) و زیردامنه admin.khabavar.ir نیز مصوب شدند.

۶. معماری داده (خلاصه اجرایی — مرجع: DB v2.0)

* Schemaها: نسخه اول یک Schema فیزیکی `public` با پیشوند ماژول (توصیه DB v2 §۲)؛ تفکیک منطقی با قواعد §۴.۲.
* استانداردها: UUID، BIGINT ریال، TIMESTAMPTZ/UTC، TEXT+CHECK وضعیت‌ها، حذف منطقی برای سوابق، Snapshot سفارش، ممنوعیت ذخیره اطلاعات کارت.
* تراکنش‌های حیاتی: Checkout (۱۰ گام DB v2 §۱۴.۱)، رزرو موجودی (قفل ردیفی، ترتیب قفل ثابت، انقضا)، پرداخت+Outbox در یک تراکنش؛ پاسخ نامشخص درگاه → استعلام، نه پرداخت مجدد.
* Migration: شماره‌گذاری‌شده up/down به تفکیک ماژول (identity→catalog→supplier→inventory→pricing→commerce→fulfillment→content→audit)؛ Migration اجراشده تغییر نمی‌کند؛ Seed جدا از Migration؛ هیچ داده فرضی عملیاتی در Seed.
* Index Strategy: طبق DB v2 §۱۵ (phone/slug/sku/order_number یکتا، استوک یکتا بر (warehouse,variant)، رزرو بر (status,expires_at)…).
* Backup: روزانه، نگهداری مستقل، رمزگذاری، تست Restore دوره‌ای؛ RPO/RTO پس از انتخاب زیرساخت ⚠️.

۷. جریان‌های کلیدی (Key Flows)

* F1 — ورود OTP: شماره → چالش OTP (هش، سقف تلاش، انقضا) → تأیید → Authorization Code یک‌بارمصرف کوتاه‌عمر → Redirect URI ثبت‌شده → نشست مستقل سرویس. رویدادهای امنیتی در Audit.
* F2 — Checkout و پرداخت: اعتبارسنجی سبد/قیمت/Variant → کنترل تأمین → ارسال/مالیات → Order+Items+PaymentIntent → پرداخت Attempt → تأیید سمت سرور (Idempotent) → تفکیک Fulfillmentها → رزرو انبار / درخواست تأیید تأمین → OrderConfirmed → اعلان.
* F3 — اعتبارسنجی تأمین‌کننده: بررسی اعتبار قیمت/موجودی (تاریخ اعتبار) → تأیید یا رد → در صورت رد: لغو قلم + بازپرداخت جزئی + ادامه سایر اقلام (Commerce سناریو B).
* F4 — بازپرداخت جزئی: درخواست → کنترل سقف (مجموع بازپرداخت ≤ قابل بازپرداخت) → ثبت با دلیل/مرجع → اجرا در Adapter درگاه (یا فرآیند دستی کنترل‌شده) → وضعیت سفارش بازتولید.
* F5 — انتشار محصول (مدل A): DRAFT → QC-01…QC-04 → QC-05 (تأیید انتشار: اطلاعات+تصاویر+تأمین+قیمت معتبر) → PUBLISHED؛ وضعیت موجودی مستقل؛ CM≤۰ → هشدار و جلوگیری از عرضه.
* F6 — مقاله↔محصول: ثبت دستی رابطه با relation_type و «دلیل ارتباط» → نمایش دوطرفه → پیوند محتوا-تجارت بدون ادعای تأییدنشده.

۸. یکپارچه‌های خارجی و الگوی Adapter

یکپارچه	قرارداد (Interface)	نسخه اول	تبدیل واقعی

درگاه پرداخت	PaymentAdapter: initiate/verify/refund	MockGateway (رفتار موفق/ناموفق/نامشخص قابل شبیه‌سازی)	پس از انتخاب درگاه ⚠️
پیامک	SmsAdapter: send(otp و notify)	Console/Mock	پس از انتخاب تأمین‌کننده ⚠️
حمل‌ونقل	CarrierAdapter: quote/label/track	Manual + Mock tracking events	قرارداد شرکت حمل ⚠️
Object Storage	StorageAdapter: put/get/sign	دیسک محلی	انتخاب سرویس ⚠️ (Master §۴ «نیازمند انتخاب»)

هر Adapter: قرارداد بسته + Mock قابل تعویض بدون تغییر ماژول‌های دامنه.

۹. معماری Frontend

* سه اپ React+TS+Vite (main/shop/magazine) + پنل admin؛ کامپوننت‌های مشترک از Design System (tokens: رنگ/تایپوگرافی/فاصله/سایه/breakpoints — Brand §۹).
* رندر: SSR/Prerendering برای صفحات SEO-محور (محصول، مقاله، دسته)؛ SPA برای جریان‌های تعاملی.
* RTL Native، اعداد فارسی، Mobile First، حالت‌های Loading/Empty/Error مشترک.
* توکن‌ها از Brand System: Midnight/Deep Navy/Sleep Blue/Lavender/Moon White/Mist/Charcoal + Success/Warning/Error؛ ممنوعیت کد رنگ مستقیم.

۱۰. امنیت

* HTTPS اجباری؛ Cookieهای HttpOnly/Secure + SameSite متناسب OIDC؛ توکن هرگز در LocalStorage؛ Redirect URI whitelist؛ CSRF روی عملیات حساس؛ Rate Limiting (OTP/ورود/API)؛ اعتبارسنجی ورودی؛ محافظت XSS/SQLi؛ رمزنگاری داده حساس در بکاپ؛ حداقل‌گرایی داده مشتری؛ تفکیک دسترسی اقتصاد/قیمت خرید به FINANCE/مدیریت؛ Audit غیرقابل حذف توسط کاربران عادی؛ ماژول هویت طوری طراحی شود که در آینده به سرویس مستقل تبدیل‌پذیر باشد (CIAM §۱۱) — اما در نسخه اول جدا نمی‌شود.

۱۱. عملکرد و مقیاس‌پذیری

* هدف: <۱۰۰۰ بازدید روزانه؛ صفحات ≤۳ ثانیه روی اینترنت ضعیف (PRD §۱.۴).
* ابزارها: Pagination همه‌جا، بهینه‌سازی تصاویر، کاهش JS غیرضروری، Cache برای محتوای عمومی (تصمیم باز Redis ⚠️)، پردازش ناهمگام وظایف غیرفوری، FTS فارسی.
* افزایش منابع فقط بر اساس اندازه‌گیری واقعی (SAD v1 §۱۴).

۱۲. استقرار

DNS/CDN → Reverse Proxy (Caddy یا Nginx ⚠️ انتخاب) → {استاتیک main/shop/magazine/admin، Backend API} → PostgreSQL → Backup Storage.
* systemd units، تفکیک Development/Staging/Production، متغیرهای محیطی بدون ذخیره در مخزن، Log ساخت‌یافته، Health Check، بکاپ زمان‌بندی‌شده، مستندات بازیابی. CDN: ArvanCloud پس از بررسی ⚠️.

۱۳. ریسک‌های معماری → مکانیزم فنی

ریسک	مکانیزم فنی در معماری

پیچیدگی تأمین ترکیبی	تفکیک Order/Fulfillment (ADR-COM-001)
تغییر قیمت تأمین‌کننده	price_valid_until + تاریخچه + Snapshot سفارش
خطای موجودی	رزرو تراکنشی + Ledger + قفل ترتیب‌دار
کاهش سود مشارکت	Economics Register + هشدار CM≤۰ + بازمحاسبه
تأخیر ارسال	رهگیری مستقل هر مرسوله + رویدادهای حمل
محتوای پزشکی نادرست	ADR-CONTENT-001 + گردش بازبینی
پیچیدگی IAM	هسته ساده استاندارد OIDC، بدون اشتراک Cookie
توسعه بیش از حد	Scope نسخه اول PRD §۷ + Quality Gates

۱۴. موارد باز معماری و معیار خروج نسخه ۲

باز (⚠️ تصمیم مالک): درگاه پرداخت، سرویس پیامک، Object Storage، Caddy/Nginx، CDN، Redis، RPO/RTO، زیردامنه پنل مدیریت، انتخاب فونت نهایی (Vazirmatn/Estedad پس از بررسی مجوز و عملکرد).
معیار خروج: مرز ماژول‌ها و قواعد وابستگی تأیید شده؛ ADR Registry مصوب؛ مدل داده نهایی (DB v2) پذیرفته؛ جریان‌های کلیدی تأیید شده؛ استانداردهای امنیتی تأیید شده → ورود به M1 گام ۳ (SQL Migrations) بدون بازطراحی.