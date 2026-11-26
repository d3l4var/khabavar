KHABAVAR — Database Architecture & Domain Design
طراحی معماری پایگاه داده و مدل داده تجاری
Project: KhabAvar / خواب‌آور
Document ID: KH-TECH-DB-001
Version: 2.0
Status: Architecture Baseline — Draft for Approval
Market: Iran
Database: PostgreSQL
Architecture: Modular Monolith
Currency: IRR (Rial)
Primary Store: shop.khabavar.ir
________________


1. اهداف معماری
پایگاه داده باید پنج نیاز اصلی را تأمین کند:
1. مدیریت فروشگاه تخصصی محصولات خواب.
2. پشتیبانی از تأمین کالا به دو روش Owned Inventory و Supplier Sourced.
3. مدیریت سفارش یکپارچه با چند Fulfillment و چند Shipment.
4. محاسبه دقیق هزینه تمام‌شده و سود مشارکت.
5. اتصال فروشگاه به مجله محتوایی و سایر بخش‌های اکوسیستم خواب‌آور.
1.1. اصول طراحی
* PostgreSQL به‌عنوان پایگاه داده اصلی.
* Modular Monolith به‌جای Microservices.
* تفکیک منطقی ماژول‌ها.
* استفاده از UUID برای شناسه موجودیت‌ها.
* ثبت زمان‌ها با TIMESTAMPTZ و ذخیره استاندارد UTC.
* نگهداری مبالغ به‌صورت عدد صحیح در واحد ریال.
* استفاده از Foreign Key و Constraints برای تضمین صحت داده.
* ثبت تاریخچه تغییرات اطلاعات مالی و تجاری.
* استفاده از Snapshot برای حفظ اطلاعات تاریخی سفارش.
* حذف منطقی برای داده‌هایی که نیازمند حفظ سوابق هستند.
* عدم ذخیره اطلاعات حساس کارت بانکی.
2. معماری ماژول‌ها
ساختار منطقی پیشنهادی:
KhabAvar Database
│
├── identity
│   ├── customers
│   ├── addresses
│   ├── roles
│   └── permissions
│
├── catalog
│   ├── products
│   ├── categories
│   ├── brands
│   ├── variants
│   ├── attributes
│   └── media
│
├── supplier
│   ├── suppliers
│   ├── supplier_products
│   ├── supplier_variants
│   └── supplier_evaluations
│
├── inventory
│   ├── warehouses
│   ├── stock_balances
│   ├── stock_movements
│   └── reservations
│
├── pricing
│   ├── cost_records
│   ├── price_records
│   └── financial_snapshots
│
├── commerce
│   ├── carts
│   ├── orders
│   ├── order_items
│   ├── payments
│   └── refunds
│
├── fulfillment
│   ├── fulfillment_orders
│   ├── fulfillment_items
│   ├── shipments
│   └── shipment_events
│
├── content
│   ├── articles
│   ├── topics
│   └── product_article_relations
│
└── audit
    ├── audit_logs
    └── domain_events
این‌ها مرزهای منطقی ماژول‌ها هستند. در نسخه اول لازم نیست هر ماژول Schema فیزیکی جداگانه داشته باشد. می‌توانیم از یک Schema اصلی با نام‌گذاری منظم جداول استفاده کنیم یا در صورت نیاز به تفکیک مالکیت، Schemaهای PostgreSQL مستقل تعریف کنیم.
پیشنهاد اولیه: یک Schema فیزیکی public و نام‌گذاری پیشونددار، تا پیچیدگی Migration و دسترسی کاهش یابد.
3. ERD — ارتباط موجودیت‌ها
erDiagram
    CUSTOMERS ||--o{ ADDRESSES : owns
    CUSTOMERS ||--o{ ORDERS : places
    CUSTOMERS ||--o{ CARTS : owns


    CATEGORIES ||--o{ PRODUCTS : contains
    PRODUCTS ||--o{ PRODUCT_VARIANTS : has
    PRODUCTS ||--o{ PRODUCT_CATEGORIES : classified
    PRODUCTS ||--o{ PRODUCT_MEDIA : displays


    SUPPLIERS ||--o{ SUPPLIER_PRODUCTS : provides
    PRODUCTS ||--o{ SUPPLIER_PRODUCTS : sourced
    PRODUCT_VARIANTS ||--o{ SUPPLIER_VARIANTS : supplied_as


    WAREHOUSES ||--o{ STOCK_BALANCES : holds
    PRODUCT_VARIANTS ||--o{ STOCK_BALANCES : stocked
    STOCK_BALANCES ||--o{ STOCK_MOVEMENTS : records


    ORDERS ||--|{ ORDER_ITEMS : contains
    ORDERS ||--o{ PAYMENTS : payment_attempts
    ORDERS ||--o{ REFUNDS : refunded


    ORDERS ||--o{ FULFILLMENT_ORDERS : fulfilled_by
    FULFILLMENT_ORDERS ||--|{ FULFILLMENT_ITEMS : contains
    FULFILLMENT_ORDERS ||--o{ SHIPMENTS : ships
    SHIPMENTS ||--o{ SHIPMENT_EVENTS : tracks


    ORDER_ITEMS ||--o{ FULFILLMENT_ITEMS : allocated
    PRODUCT_VARIANTS ||--o{ INVENTORY_RESERVATIONS : reserved


    PRODUCTS ||--o{ PRODUCT_ARTICLES : linked
    ARTICLES ||--o{ PRODUCT_ARTICLES : references
نکته: برای هر رابطه‌ای که در ERD به‌صورت یک‌به‌چند نمایش داده شده، قواعد حذف، محدودیت تعداد و وضعیت عملیاتی باید در Migration مشخص شوند.
4. استانداردهای عمومی داده
4.1. شناسه‌ها
شناسه اصلی:
UUID
برای شناسه‌های عمومی قابل مشاهده مانند شماره سفارش، از شناسه تجاری جداگانه استفاده می‌کنیم.
نمونه:
KHA-ORD-2026-000001
شماره سفارش باید یکتا باشد و از UUID داخلی مستقل بماند.
4.2. مبالغ مالی
تمام مبالغ ذخیره‌شده در دیتابیس با واحد ریال هستند.
نوع پیشنهادی:
BIGINT
مثال:
نمایش مشتری
	مقدار ذخیره‌شده
	۱۰۰ هزار تومان
	۱٬۰۰۰٬۰۰۰ ریال
	۵۰۰ هزار تومان
	۵٬۰۰۰٬۰۰۰ ریال
	یک میلیون تومان
	۱۰٬۰۰۰٬۰۰۰ ریال
	تبدیل ریال و تومان فقط در لایه Presentation انجام شود.
4.3. زمان
* تمام زمان‌ها: TIMESTAMPTZ
* ذخیره‌سازی: UTC
* منطقه زمانی نمایش: Asia/Tehran
* تاریخ سفارش و زمان رویدادها نباید با تغییر تنظیمات منطقه زمانی تغییر کنند.
4.4. وضعیت‌ها
وضعیت‌های مهم باید در لایه Domain تعریف شوند و در دیتابیس با محدودیت‌های معتبر کنترل شوند.
پیشنهاد: استفاده از TEXT به همراه CHECK Constraint برای وضعیت‌هایی که احتمال تغییر دارند، به‌جای وابستگی زودهنگام به PostgreSQL ENUM.
5. ماژول Identity
5.1. customers
ستون
	نوع
	توضیح
	id
	UUID
	شناسه
	phone
	VARCHAR(20)
	شماره تلفن
	email
	VARCHAR(255)
	ایمیل، اختیاری
	full_name
	VARCHAR(200)
	نام مشتری
	status
	TEXT
	وضعیت
	phone_verified_at
	TIMESTAMPTZ
	تأیید تلفن
	created_at
	TIMESTAMPTZ
	ایجاد
	updated_at
	TIMESTAMPTZ
	تغییر
	قواعد:
* شماره تلفن در صورت وجود باید یکتا باشد.
* شماره تلفن باید با قالب استاندارد داخلی ذخیره شود.
* احراز هویت با OTP می‌تواند در مرحله پیاده‌سازی اضافه شود.
* OTP خام نباید ذخیره شود.
* اطلاعات حساس غیرضروری نباید جمع‌آوری شوند.
5.2. customer_addresses
ستون
	نوع
	توضیح
	id
	UUID
	شناسه
	customer_id
	UUID
	مشتری
	province
	VARCHAR
	استان
	city
	VARCHAR
	شهر
	postal_code
	VARCHAR
	کد پستی
	address_line
	TEXT
	نشانی
	plaque
	VARCHAR
	پلاک
	unit
	VARCHAR
	واحد
	recipient_name
	VARCHAR
	تحویل‌گیرنده
	recipient_phone
	VARCHAR
	تلفن تحویل‌گیرنده
	is_default
	BOOLEAN
	پیش‌فرض
	آدرس باید هنگام ثبت سفارش Snapshot شود تا تغییر بعدی آدرس مشتری روی سفارش قبلی تأثیر نگذارد.
6. ماژول Catalog
6.1. categories
فیلدها:
* id
* parent_id
* name
* slug
* description
* image_id
* sort_order
* status
* created_at
* updated_at
ساختار درختی با parent_id پیاده‌سازی می‌شود.
هر محصول می‌تواند در چند دسته‌بندی قرار گیرد.
6.2. products
ستون
	نوع
	id
	UUID
	title
	VARCHAR(255)
	slug
	VARCHAR(255)
	brand_id
	UUID
	short_description
	TEXT
	description
	TEXT
	manufacturer
	VARCHAR(255)
	country_of_origin
	VARCHAR(100)
	status
	TEXT
	published_at
	TIMESTAMPTZ
	created_at
	TIMESTAMPTZ
	updated_at
	TIMESTAMPTZ
	وضعیت:
* DRAFT
* DATA_REVIEW
* APPROVED
* PUBLISHED
* PAUSED
* DISCONTINUED
6.3. product_variants
هر Variant قابل فروش شناسه و SKU مستقل دارد.
ستون
	نوع
	id
	UUID
	product_id
	UUID
	sku
	VARCHAR(100)
	barcode
	VARCHAR(100)
	attributes
	JSONB
	weight_grams
	INTEGER
	length_mm
	INTEGER
	width_mm
	INTEGER
	height_mm
	INTEGER
	status
	TEXT
	created_at
	TIMESTAMPTZ
	قواعد:
* SKU یکتا باشد.
* ابعاد و وزن غیرمنفی باشند.
* ویژگی‌های اصلی و فیلترپذیر در ساختار Attribute ذخیره شوند.
* JSONB فقط برای مشخصات انعطاف‌پذیر و غیرهسته‌ای استفاده شود.
6.4. Product Attributes
جداول پیشنهادی:
* attribute_definitions
* attribute_options
* product_attribute_values
این مدل امکان تعریف ویژگی‌های متفاوت برای بالش، ملحفه، چشم‌بند و تجهیزات هوشمند را فراهم می‌کند.
مثال:
دسته
	ویژگی
	بالش
	ارتفاع
	بالش
	جنس مواد داخلی
	بالش
	میزان نرمی
	محافظ تشک
	ابعاد
	محافظ تشک
	ضدآب بودن
	چشم‌بند
	جنس
	چراغ خواب
	شدت نور
	برای مقادیر عددی، واحد اندازه‌گیری باید مشخص باشد.
6.5. product_media
* id
* product_id
* variant_id
* media_type
* storage_key
* alt_text
* sort_order
* created_at
فایل تصاویر در Object Storage یا فضای فایل مستقل ذخیره می‌شود؛ دیتابیس فقط آدرس یا کلید فایل را نگهداری می‌کند.
7. ماژول Supplier
7.1. suppliers
* id
* name
* supplier_type
* registration_number
* contact_name
* phone
* email
* address
* website
* status
* created_at
* updated_at
7.2. supplier_products
ارتباط بین تأمین‌کننده و محصول.
* id
* supplier_id
* product_id
* supplier_product_code
* minimum_order_quantity
* lead_time_days
* status
7.3. supplier_variants
* id
* supplier_product_id
* variant_id
* supplier_sku
* purchase_price
* currency
* available_quantity
* last_stock_check_at
* price_valid_until
* last_price_check_at
* status
قواعد مهم:
* قیمت تأمین‌کننده تاریخ اعتبار دارد.
* موجودی تأمین‌کننده نباید با موجودی انبار خواب‌آور یکی شود.
* هر تغییر قیمت باید در تاریخچه ثبت شود.
* تأمین‌کننده می‌تواند یک Variant را با چند شرایط تجاری عرضه کند.
* برای تأمین یک Variant از چند تأمین‌کننده باید امکان ثبت چند منبع وجود داشته باشد.
7.4. supplier_evaluations
* id
* supplier_id
* quality_score
* pricing_score
* availability_score
* warranty_score
* documentation_score
* flexibility_score
* logistics_score
* communication_score
* total_score
* evaluation_date
* reviewer_id
* notes
وزن‌ها باید در یک نسخه سیاست ارزیابی ثبت شوند تا تغییر وزن‌ها امتیازهای تاریخی را مخدوش نکند.
8. ماژول Inventory
8.1. warehouses
* id
* name
* warehouse_code
* address
* status
8.2. stock_balances
* id
* warehouse_id
* variant_id
* on_hand_quantity
* reserved_quantity
* updated_at
قید یکتا:
UNIQUE (warehouse_id, variant_id)
موجودی قابل فروش:
Available = On Hand - Reserved
مقادیر نباید منفی شوند.
8.3. stock_movements
* id
* warehouse_id
* variant_id
* movement_type
* quantity
* reference_type
* reference_id
* created_at
* created_by
انواع حرکت:
* RECEIPT
* SALE
* RESERVATION
* RELEASE
* RETURN
* ADJUSTMENT
* DAMAGE
* TRANSFER
این جدول دفتر ثبت تغییرات موجودی است و نباید برای اصلاح اشتباهات، رکوردهای قبلی آن حذف شوند. اصلاح باید با ثبت حرکت معکوس انجام شود.
8.4. inventory_reservations
* id
* order_item_id
* variant_id
* warehouse_id
* supplier_id
* quantity
* status
* expires_at
* created_at
رزرو موجودی باید منبع مشخص داشته باشد.
برای موجودی انبار، رزرو واقعی در stock_balances انجام می‌شود.
برای موجودی تأمین‌کننده، وضعیت تأیید تأمین باید جداگانه ثبت شود و نباید بدون تعهد معتبر، موجودی قطعی تلقی شود.
9. ماژول Pricing
9.1. product_cost_records
* id
* variant_id
* supply_type
* supplier_id
* purchase_cost
* inbound_freight
* inspection_cost
* packaging_cost
* fulfillment_cost
* estimated_payment_fee
* estimated_shipping_subsidy
* expected_return_cost
* currency
* valid_from
* valid_until
* created_at
* created_by
این جدول تاریخچه محاسبات هزینه را حفظ می‌کند.
9.2. product_price_records
* id
* variant_id
* selling_price
* discount_policy
* target_contribution_rate
* calculated_contribution
* currency
* valid_from
* valid_until
* approved_by
* created_at
قیمت فروش باید قابل تأیید توسط نقش مجاز باشد.
9.3. order_financial_snapshots
* id
* order_id
* order_item_id
* net_revenue
* cogs
* variable_costs
* contribution
* contribution_rate
* calculation_version
* created_at
این Snapshot مبنای گزارش سودآوری سفارش‌های گذشته است.
نکته مهم: محاسبه سود مشارکت باید بتواند هزینه‌های مشترک سفارش را با روش تخصیص مشخص محاسبه کند. هزینه‌هایی که در سطح Order هستند نباید بدون قاعده به هر قلم نسبت داده شوند.
10. ماژول Commerce
10.1. carts
* id
* customer_id
* session_id
* status
* currency
* created_at
* updated_at
* expires_at
10.2. cart_items
* id
* cart_id
* variant_id
* quantity
* created_at
* updated_at
قیمت نهایی در زمان Checkout از قیمت معتبر روز محاسبه می‌شود؛ قیمت قدیمی سبد نباید به‌صورت خودکار تضمین شود.
10.3. orders
ستون
	نوع
	id
	UUID
	order_number
	VARCHAR(50)
	customer_id
	UUID
	status
	TEXT
	currency
	CHAR(3)
	subtotal
	BIGINT
	discount_total
	BIGINT
	shipping_total
	BIGINT
	tax_total
	BIGINT
	grand_total
	BIGINT
	paid_total
	BIGINT
	refunded_total
	BIGINT
	shipping_address_snapshot
	JSONB
	created_at
	TIMESTAMPTZ
	updated_at
	TIMESTAMPTZ
	قید:
* order_number یکتا باشد.
* مبالغ منفی نباشند.
* مبالغ محاسباتی سفارش با اقلام و هزینه‌ها سازگار باشند.
10.4. order_items
* id
* order_id
* product_id
* variant_id
* sku_snapshot
* title_snapshot
* attributes_snapshot
* quantity
* unit_price
* discount_amount
* tax_amount
* line_total
* supply_type_snapshot
* created_at
اطلاعات Snapshot باید مستقل از تغییرات بعدی کاتالوگ باقی بماند.
10.5. payments
پیشنهاد می‌شود Payment Intent و Payment Attempt از یکدیگر تفکیک شوند.
payment_intents
* id
* order_id
* amount
* currency
* status
* expires_at
* created_at
payment_attempts
* id
* payment_intent_id
* provider
* provider_reference
* amount
* status
* failure_code
* created_at
* completed_at
این تفکیک اجازه می‌دهد مشتری یک پرداخت برای سفارش داشته باشد، اما در صورت خطای درگاه بتواند تلاش پرداخت را تکرار کند.
قید تجاری: برای یک سفارش در نسخه اول فقط یک پرداخت موفق اصلی مجاز است؛ پرداخت‌های تکراری موفق باید شناسایی و تعیین تکلیف مالی شوند.
10.6. refunds
* id
* order_id
* payment_attempt_id
* order_item_id
* amount
* reason
* status
* provider_reference
* created_at
* completed_at
بازپرداخت باید به تراکنش موفق اصلی متصل شود.
11. ماژول Fulfillment
11.1. fulfillment_orders
* id
* order_id
* source_type
* warehouse_id
* supplier_id
* status
* estimated_ready_at
* estimated_delivery_at
* created_at
* updated_at
source_type:
* OWNED_INVENTORY
* SUPPLIER_SOURCED
11.2. fulfillment_items
* id
* fulfillment_order_id
* order_item_id
* quantity
* status
قید:
* مجموع تعداد تخصیص‌یافته به Fulfillmentها نباید از تعداد قلم سفارش بیشتر باشد.
* تخصیص باید در تراکنش کنترل شود.
11.3. shipments
* id
* fulfillment_order_id
* carrier
* service_type
* tracking_number
* shipping_cost
* status
* shipped_at
* delivered_at
* created_at
هر Fulfillment می‌تواند چند Shipment داشته باشد.
11.4. shipment_items
* id
* shipment_id
* fulfillment_item_id
* quantity
این ساختار اجازه می‌دهد یک قلم سفارش نیز در صورت نیاز در چند بسته ارسال شود.
11.5. shipment_events
* id
* shipment_id
* event_code
* description
* occurred_at
* source
* external_reference
* created_at
تمام رویدادهای رهگیری به ترتیب زمانی ثبت می‌شوند.
12. ماژول Content
مجله خواب‌آور یک بخش مستقل و مهم از اکوسیستم است و باید در مدل داده دیده شود.
12.1. articles
* id
* title
* slug
* summary
* body
* language
* source_type
* source_url
* author_id
* editor_id
* status
* published_at
* created_at
* updated_at
12.2. topics
نمونه موضوعات:
* Sleep Science
* Insomnia
* Nutrition
* Medication
* Drinks
* Psychology
* Metabolism
* Physical Health
* Mental Health
* Lifestyle
* Culture & Traditions
12.3. product_article_relations
* id
* product_id
* article_id
* relation_type
* created_at
نوع ارتباط:
* RELATED
* EDUCATIONAL
* PRODUCT_GUIDE
* COMPARISON
ارتباط محصول و مقاله چندبه‌چند است.
اصل مهم: محتوای علمی و سلامت‌محور نباید تحت تأثیر منفعت تجاری محصول دست‌کاری شود.
13. ماژول Audit
13.1. audit_logs
* id
* actor_id
* actor_type
* action
* entity_type
* entity_id
* old_values
* new_values
* ip_address
* created_at
برای عملیات حساس مانند:
* تغییر قیمت
* تغییر هزینه خرید
* تأیید تأمین‌کننده
* تغییر موجودی
* بازپرداخت
* تغییر وضعیت سفارش
* تغییر شرایط ارسال
13.2. domain_events
* id
* aggregate_type
* aggregate_id
* event_type
* payload
* occurred_at
* published_at
* retry_count
Transactional Outbox برای انتشار قابل اتکای رویدادها استفاده می‌شود.
14. تراکنش‌های حیاتی
14.1. Checkout
ترتیب عملیات:
1. اعتبارسنجی سبد.
2. اعتبارسنجی قیمت‌ها.
3. اعتبارسنجی Variantها.
4. کنترل تأمین.
5. محاسبه هزینه ارسال.
6. محاسبه مالیات در صورت شمول.
7. ایجاد Order.
8. ایجاد Order Items.
9. ایجاد Payment Intent.
10. ثبت اطلاعات لازم برای ادامه فرآیند.
پس از موفقیت پرداخت:
1. تأیید تراکنش در سمت سرور.
2. جلوگیری از پردازش مجدد پرداخت.
3. ثبت موفقیت پرداخت.
4. ایجاد یا فعال‌کردن Fulfillmentها.
5. رزرو موجودی اختصاصی.
6. درخواست تأیید تأمین از تأمین‌کنندگان.
7. انتشار رویداد OrderConfirmed.
اگر پاسخ درگاه نامشخص باشد، سفارش نباید بدون استعلام وضعیت تراکنش دوباره پرداخت شود.
14.2. رزرو موجودی
برای جلوگیری از فروش بیش از موجودی:
* رزرو با Transaction انجام شود.
* ردیف موجودی با قفل مناسب خوانده شود.
* مقدار موجودی قابل فروش بررسی شود.
* رزرو فقط در صورت وجود ظرفیت انجام شود.
* زمان انقضای رزرو مشخص باشد.
* آزادسازی رزرو با ثبت حرکت موجودی انجام شود.
برای رزرو هم‌زمان چند Variant باید ترتیب قفل‌گذاری ثابت باشد تا احتمال Deadlock کاهش یابد.
14.3. پرداخت و Outbox
تأیید پرداخت و ثبت رویداد مربوط به آن باید در یک تراکنش دیتابیس انجام شود.
ارسال رویداد به سرویس‌های دیگر نباید در میانه تراکنش اصلی انجام شود.
15. Index Strategy
Indexهای اولیه:
جدول
	Index
	customers
	phone UNIQUE
	products
	slug UNIQUE
	product_variants
	sku UNIQUE
	categories
	parent_id
	orders
	order_number UNIQUE
	orders
	customer_id, created_at
	order_items
	order_id
	supplier_products
	supplier_id, product_id
	stock_balances
	warehouse_id, variant_id UNIQUE
	inventory_reservations
	status, expires_at
	fulfillment_orders
	order_id, status
	shipments
	tracking_number
	shipment_events
	shipment_id, occurred_at
	product_cost_records
	variant_id, valid_from
	articles
	slug UNIQUE
	articles
	status, published_at
	برای جست‌وجوی فارسی:
* ابتدا PostgreSQL Full Text Search بررسی شود.
* نرمال‌سازی ی و ک عربی و فارسی انجام شود.
* نیم‌فاصله و شکل‌های مختلف نوشتار بررسی شوند.
* در صورت ناکافی بودن نتایج، ابزار جست‌وجوی مستقل در مرحله بعد اضافه شود.
16. Migration Strategy
Migrationها باید شماره‌گذاری و قابل تکرار باشند.
ساختار پیشنهادی:
migrations/
├── 000001_identity.up.sql
├── 000001_identity.down.sql
├── 000002_catalog.up.sql
├── 000002_catalog.down.sql
├── 000003_supplier.up.sql
├── 000003_supplier.down.sql
├── 000004_inventory.up.sql
├── 000004_inventory.down.sql
├── 000005_pricing.up.sql
├── 000005_pricing.down.sql
├── 000006_commerce.up.sql
├── 000006_commerce.down.sql
├── 000007_fulfillment.up.sql
├── 000007_fulfillment.down.sql
├── 000008_content.up.sql
├── 000008_content.down.sql
└── 000009_audit.up.sql
قواعد:
* Migration اجراشده نباید بی‌دلیل تغییر کند.
* اصلاح ساختار با Migration جدید انجام شود.
* Migrationهای مخرب نیازمند بررسی و Backup هستند.
* داده‌های واقعی نباید با Down Migration ناخواسته حذف شوند.
* Migration و Seed Data از هم جدا باشند.
17. Seed Data
داده‌های اولیه پیشنهادی:
* دسته‌بندی‌های محصولات خواب
* ویژگی‌های استاندارد دسته‌ها
* انواع تأمین
* وضعیت‌های سفارش
* وضعیت‌های Fulfillment
* وضعیت‌های Shipment
* نقش‌های مدیریتی
* انواع ارتباط مقاله و محصول
هیچ تأمین‌کننده، موجودی یا قیمت فرضی نباید به‌عنوان داده عملیاتی واقعی وارد Seed شود.
18. امنیت و دسترسی دیتابیس
نقش‌های پیشنهادی:
Role
	دسترسی
	db_migrator
	اجرای Migration
	app_runtime
	عملیات موردنیاز برنامه
	reporting_reader
	خواندن داده‌های گزارش
	backup_operator
	عملیات پشتیبان‌گیری
	قواعد:
* دسترسی Runtime حداقل باشد.
* رمز عبور دیتابیس در مخزن کد ذخیره نشود.
* اطلاعات پرداخت حساس ذخیره نشوند.
* اطلاعات مشتری به نقش‌های غیرمرتبط نمایش داده نشوند.
* اطلاعات قیمت خرید و سود مشارکت محدود به نقش مالی و مدیریت باشد.
* Audit Log از دستکاری عادی کاربران جلوگیری کند.
19. Backup & Recovery
برای نسخه اول:
* Backup روزانه.
* نگهداری نسخه‌های پشتیبان در محل مستقل.
* رمزگذاری Backup.
* آزمایش دوره‌ای Restore.
* ثبت زمان آخرین Backup موفق.
* برنامه بازیابی در صورت خرابی دیتابیس.
مقادیر RPO و RTO باید با توجه به زیرساخت و هزینه عملیاتی تصویب شوند.
20. برنامه پیاده‌سازی
DB-P1 — Core Foundation
* Identity
* Catalog
* Categories
* Product Variants
* Product Attributes
DB-P2 — Supply & Inventory
* Suppliers
* Supplier Products
* Warehouses
* Stock Balances
* Reservations
* Stock Movements
DB-P3 — Pricing
* Cost Records
* Price Records
* Financial Snapshots
DB-P4 — Commerce
* Carts
* Orders
* Order Items
* Payments
* Refunds
DB-P5 — Fulfillment
* Fulfillment Orders
* Fulfillment Items
* Shipments
* Shipment Events
DB-P6 — Content & Integration
* Articles
* Topics
* Product Article Relations
DB-P7 — Audit & Reporting
* Audit Logs
* Domain Events
* Financial Reporting
21. معیارهای پذیرش نهایی طراحی دیتابیس
* مدل داده از تأمین ترکیبی پشتیبانی کند.
* یک محصول بتواند چند Variant داشته باشد.
* یک Variant بتواند چند تأمین‌کننده داشته باشد.
* موجودی انبار و موجودی تأمین‌کننده مستقل باشند.
* رزرو موجودی هم‌زمان امن باشد.
* یک سفارش بتواند چند Fulfillment داشته باشد.
* یک Fulfillment بتواند چند Shipment داشته باشد.
* یک پرداخت اصلی برای سفارش پشتیبانی شود.
* تلاش‌های پرداخت قابل تکرار و رهگیری باشند.
* بازپرداخت جزئی امکان‌پذیر باشد.
* اطلاعات تاریخی سفارش Snapshot شوند.
* قیمت‌گذاری و سود مشارکت قابل ممیزی باشند.
* مجله و فروشگاه امکان ارتباط چندبه‌چند داشته باشند.
* تغییرات مهم در Audit ثبت شوند.
* Migrationها قابل مدیریت و نسخه‌بندی باشند.
* Backup و Restore قابل آزمایش باشند.
________________


22. تصمیم‌های معماری تثبیت‌شده
شناسه
	تصمیم
	وضعیت
	ADR-DB-001
	PostgreSQL
	ACCEPTED
	ADR-DB-002
	Modular Monolith
	PROPOSED
	ADR-DB-003
	UUID identifiers
	PROPOSED
	ADR-COM-001
	یک سفارش و پرداخت، چند مرسوله
	ACCEPTED
	ADR-COM-002
	هزینه تمام‌شده و سود مشارکت
	ACCEPTED
	ADR-SUP-001
	Mixed Supply
	ACCEPTED
	ADR-CAT-001
	حوزه محصولات Sleep
	ACCEPTED
	موارد PROPOSED تا زمان تصویب نهایی نباید به‌عنوان تصمیم قطعی معماری تلقی شوند.
پایان سند — KHABAVAR DATABASE ARCHITECTURE v2.0