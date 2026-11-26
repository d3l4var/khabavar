KHABAVAR — DATABASE ARCHITECTURE
نسخه 1.0
1. اصول طراحی
* PostgreSQL به‌عنوان پایگاه داده اصلی.
* استفاده از UUID برای شناسه موجودیت‌ها.
* تفکیک منطقی ماژول‌ها.
* حفظ یکپارچگی تراکنش‌ها.
* ثبت رخدادهای مهم.
* پشتیبانی از توسعه آینده.
* جلوگیری از وابستگی غیرضروری بین ماژول‌ها.
2. ساختار منطقی
پیشنهاد می‌شود از Schemaهای منطقی زیر استفاده شود:
PostgreSQL
│
├── identity
├── commerce
├── content
├── media
├── system
└── analytics
این تفکیک منطقی است؛ در نسخه اول می‌توان همه آن‌ها را در یک PostgreSQL Database نگهداری کرد.
3. Identity Schema
users
* id
* email
* mobile
* password_hash
* display_name
* status
* created_at
* updated_at
user_sessions
* id
* user_id
* session_hash
* expires_at
* created_at
roles
* id
* name
* description
permissions
* id
* resource
* action
user_roles
* user_id
* role_id
role_permissions
* role_id
* permission_id
addresses
* id
* user_id
* province
* city
* postal_code
* address_line
* recipient_name
* recipient_phone
* is_default
نکته: اطلاعات تماس باید با سیاست مشخصی برای اعتبارسنجی و حفاظت از داده نگهداری شوند.
4. Commerce Schema
products
* id
* slug
* title
* description
* brand
* status
* product_type
* created_at
* updated_at
product_variants
* id
* product_id
* sku
* title
* price
* compare_at_price
* weight
* status
product_categories
* id
* parent_id
* title
* slug
* description
* sort_order
product_category_links
* product_id
* category_id
product_attributes
* id
* name
* code
* data_type
product_attribute_values
* id
* product_id
* variant_id
* attribute_id
* value
این ساختار امکان تعریف ویژگی‌های متفاوت برای انواع کالا را فراهم می‌کند.
مثلاً برای یک بالش، ابعاد و جنس؛ برای یک چراغ خواب، توان و نوع نور؛ و برای یک محصول دیگر، مشخصات متفاوتی تعریف می‌شود.
product_images
* id
* product_id
* variant_id
* media_id
* alt_text
* sort_order
inventory
* id
* variant_id
* warehouse_id
* quantity_on_hand
* quantity_reserved
* updated_at
warehouses
* id
* name
* location
* status
5. Cart Schema
carts
* id
* user_id
* anonymous_token
* status
* created_at
* updated_at
cart_items
* id
* cart_id
* variant_id
* quantity
* unit_price_snapshot
قیمت نهایی باید هنگام ثبت سفارش مجدداً از منبع معتبر بررسی شود.
6. Orders Schema
orders
* id
* order_number
* user_id
* status
* subtotal
* discount_total
* shipping_total
* grand_total
* currency
* address_snapshot
* created_at
* updated_at
order_items
* id
* order_id
* variant_id
* product_title_snapshot
* sku_snapshot
* quantity
* unit_price
* line_total
Snapshot باعث می‌شود تغییر نام یا قیمت محصول، اطلاعات سفارش قدیمی را تغییر ندهد.
7. Payments Schema
payments
* id
* order_id
* provider
* provider_reference
* amount
* status
* verified_at
* created_at
payment_events
* id
* payment_id
* event_type
* event_reference
* payload_metadata
* created_at
اطلاعات حساس کارت بانکی نباید در پایگاه داده فروشگاه ذخیره شود.
8. Shipping Schema
shipments
* id
* order_id
* provider
* tracking_code
* status
* shipped_at
* delivered_at
shipping_events
* id
* shipment_id
* status
* description
* occurred_at
9. Content Schema
articles
* id
* slug
* title
* summary
* body
* status
* author_id
* reviewer_id
* published_at
* created_at
* updated_at
article_categories
* id
* parent_id
* title
* slug
* description
article_category_links
* article_id
* category_id
article_tags
* id
* name
* slug
article_tag_links
* article_id
* tag_id
article_sources
* id
* title
* publisher
* source_url
* source_type
* publication_date
* access_date
* rights_status
article_references
* article_id
* source_id
* reference_text
article_translations
* id
* article_id
* source_language
* target_language
* original_title
* original_url
* translation_status
editorial_reviews
* id
* article_id
* reviewer_id
* review_type
* status
* notes
* reviewed_at
10. ارتباط تجارت و محتوا
article_products
* article_id
* product_id
* relation_type
* sort_order
product_articles
در صورت نیاز می‌توان ارتباط معکوس را از همان جدول article_products استخراج کرد و از ذخیره تکراری جلوگیری نمود.
11. Media Schema
media_assets
* id
* storage_key
* mime_type
* file_size
* width
* height
* alt_text
* created_at
فایل‌های رسانه‌ای در Storage نگهداری می‌شوند و فقط اطلاعات و شناسه آن‌ها در دیتابیس قرار می‌گیرد.
12. System Schema
audit_logs
* id
* actor_id
* action
* resource_type
* resource_id
* metadata
* created_at
notifications
* id
* user_id
* channel
* status
* payload
* created_at
settings
* key
* value
* updated_at
event_outbox
* id
* event_type
* aggregate_id
* payload
* status
* created_at
* processed_at
13. قواعد حیاتی داده
1. سفارش پس از ثبت باید Snapshot مستقل از مشخصات محصول داشته باشد.
2. عملیات پرداخت باید Idempotent باشد.
3. رزرو و کاهش موجودی باید در تراکنش امن انجام شود.
4. سفارش و پرداخت نباید صرفاً با اطلاعات مرورگر مشتری تأیید شوند.
5. حذف اطلاعات سفارش نباید به‌صورت عادی مجاز باشد.
6. اطلاعات مالی باید قابلیت تطبیق و حسابرسی داشته باشند.
7. وضعیت‌ها باید با State Machine مشخص مدیریت شوند.
8. داده‌های شخصی باید حداقل‌گرایی و کنترل دسترسی داشته باشند.
14. تصمیم‌های باز
* نوع نهایی محصولات
* تعداد انبارها
* سیاست نگهداری موجودی
* نیاز به چند فروشنده در آینده
* سیاست مالیات و فاکتور
* سیاست نگهداری اطلاعات مشتری
* سیاست حذف و ناشناس‌سازی داده‌ها
15. معیار پذیرش
* Migrationهای قابل اجرا و برگشت‌پذیر در صورت امکان
* Foreign Key و Indexهای مناسب
* محدودیت‌های یکتایی
* تست تراکنش سفارش
* تست رزرو موجودی
* تست پرداخت تکراری
* تست بازیابی Backup
* مستندات ERD
وضعیت: طرح مفهومی پایگاه داده؛ پیش از پیاده‌سازی باید به DDL و ERD نهایی تبدیل شود.