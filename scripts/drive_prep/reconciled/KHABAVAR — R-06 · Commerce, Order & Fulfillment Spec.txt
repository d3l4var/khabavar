یادداشت تطبیق (2026-11-25) — اصلاح آرایشی سجل KH-INDEX-001 (مغایرت ۱۹): ارجاع‌های «.md» بخش «اثر این تصمیم‌ها بر اسناد قبلی» به شناسه‌های رسمی R-16/R-13/R-19/R-04 اصلاح شد.
* نسخه پیشین در پوشه «آرشیو — نسخه‌های پیشین» محفوظ است.
________________

KHABAVAR — Commerce, Order & Fulfillment Specification
مشخصات اجرایی سفارش، پرداخت، تأمین و ارسال
Project: KhabAvar / خواب‌آور
Version: 1.0
Status: Draft — Architecture Baseline
Scope: Store, Commerce Operations, Inventory, Supplier Fulfillment
Market: Iran
________________


1. تصمیم‌های قطعی تجاری (Architecture Decision Records)
ADR-COM-001 — یک سفارش و پرداخت، چند مرسوله
وضعیت: ACCEPTED
تصمیم
مشتری در فروشگاه خواب‌آور یک سبد خرید ایجاد می‌کند، یک سفارش نهایی ثبت می‌کند و یک پرداخت برای کل سفارش انجام می‌دهد.
پس از ثبت سفارش، سیستم بر اساس منبع تأمین و محل نگهداری کالا، سفارش را به یک یا چند بخش اجرایی (Fulfillment) تقسیم می‌کند.
هر Fulfillment می‌تواند:
* از انبار اختصاصی خواب‌آور ارسال شود.
* توسط یک تأمین‌کننده ارسال شود.
* دارای زمان آماده‌سازی مستقل باشد.
* دارای مرسوله و کد رهگیری مستقل باشد.
* وضعیت ارسال و تحویل مستقل داشته باشد.
تجربه مشتری
مشتری باید بتواند:
* یک شماره سفارش اصلی داشته باشد.
* مبلغ نهایی کل سفارش را یک‌بار پرداخت کند.
* تمام اقلام سفارش را در یک صفحه مشاهده کند.
* وضعیت هر کالا و مرسوله را جداگانه ببیند.
* زمان تحویل تخمینی هر مرسوله را مشاهده کند.
* در صورت ارسال چندمرسوله‌ای، تعداد مرسوله‌ها را بداند.
* درخواست پشتیبانی را در سطح سفارش یا قلم سفارش ثبت کند.
محدودیت
یک سفارش واحد لزوماً به معنی یک بسته فیزیکی یا یک زمان تحویل نیست.
________________


ADR-COM-002 — قیمت‌گذاری بر مبنای هزینه تمام‌شده و سود مشارکت
وضعیت: ACCEPTED
تصمیم
قیمت‌گذاری محصولات بر اساس اقتصاد واحد (Unit Economics) انجام می‌شود.
مبنای محاسبه:
* هزینه خرید
* هزینه حمل ورودی
* هزینه کنترل کیفیت
* هزینه بسته‌بندی
* هزینه پردازش سفارش
* کارمزد پرداخت
* سهم هزینه ارسال
* هزینه مورد انتظار مرجوعی
* سایر هزینه‌های متغیر
* سود مشارکت هدف
اصل
درصد افزوده ثابت روی قیمت خرید، مبنای اصلی قیمت‌گذاری نیست.
نتیجه فنی
سیستم باید امکان محاسبه و ذخیره موارد زیر را داشته باشد:
* قیمت خرید و سابقه تغییرات
* هزینه تمام‌شده
* قیمت فروش
* تخفیف
* درآمد خالص
* سود ناخالص
* سود مشارکت
* نرخ سود مشارکت
* تاریخ اعتبار محاسبات
* نسخه سیاست قیمت‌گذاری
تمام محاسبات باید قابل ممیزی باشند.
________________


2. مدل مفهومی سفارش
ساختار اصلی:
Customer
→ Cart
→ Order
→ Order Items
→ Payment
→ Fulfillment Groups
→ Shipments
→ Delivery
Order
سفارش اصلی متعلق به مشتری است و اطلاعات مالی و وضعیت کلی خرید را نگهداری می‌کند.
Order Item
هر قلم سفارش یک Variant مشخص با تعداد، قیمت و شرایط فروش ثبت‌شده در زمان سفارش است.
Fulfillment
گروه اجرایی برای آماده‌سازی و ارسال مجموعه‌ای از اقلام سفارش است.
Shipment
مرسوله فیزیکی که ممکن است شامل یک یا چند قلم از یک Fulfillment باشد.
Payment
پرداخت مربوط به سفارش اصلی است و نباید برای هر تأمین‌کننده به‌صورت مستقل از مشتری دریافت شود.
________________


3. قواعد تفکیک Fulfillment
سیستم باید اقلام سفارش را با توجه به این عوامل گروه‌بندی کند:
1. منبع تأمین
2. انبار یا محل ارسال
3. تأمین‌کننده
4. زمان آماده‌سازی
5. محدودیت‌های حمل
6. نوع بسته‌بندی
7. سیاست ارسال
مثال:
مشتری سه محصول خریداری کرده است:
محصول
	منبع
	زمان آماده‌سازی فرضی
	بالش مموری‌فوم
	انبار خواب‌آور
	۱ روز
	محافظ تشک
	تأمین‌کننده A
	۲ روز
	چشم‌بند
	تأمین‌کننده B
	۳ روز
	سیستم یک Order ایجاد می‌کند و سه Fulfillment مستقل می‌سازد.
هر Fulfillment پس از آماده‌شدن می‌تواند به Shipment تبدیل شود.
زمان‌های فوق صرفاً برای نمایش مدل هستند و زمان واقعی باید از اطلاعات معتبر انبار یا تأمین‌کننده دریافت شود.
________________


4. چرخه عمر سفارش
وضعیت‌های پیشنهادی Order:
* DRAFT
* PENDING_PAYMENT
* PAID
* CONFIRMED
* PROCESSING
* PARTIALLY_FULFILLED
* PARTIALLY_SHIPPED
* SHIPPED
* PARTIALLY_DELIVERED
* DELIVERED
* CANCELLATION_REQUESTED
* PARTIALLY_CANCELLED
* CANCELLED
* REFUND_PENDING
* PARTIALLY_REFUNDED
* REFUNDED
* CLOSED
وضعیت سفارش اصلی باید از وضعیت اقلام، پرداخت و Fulfillmentها قابل استخراج باشد؛ اما تغییرات آن باید در تاریخچه سفارش ثبت شوند.
4.1. چرخه عمر Fulfillment
* PENDING
* SOURCE_VALIDATION
* CONFIRMED
* PICKING
* PACKING
* READY_TO_SHIP
* HANDED_TO_CARRIER
* IN_TRANSIT
* DELIVERED
* DELIVERY_FAILED
* RETURN_REQUESTED
* RETURNED
* CANCELLED
4.2. چرخه عمر Shipment
* CREATED
* LABEL_CREATED
* PICKED_UP
* IN_TRANSIT
* OUT_FOR_DELIVERY
* DELIVERED
* DELIVERY_FAILED
* RETURN_TO_SENDER
* LOST
* CANCELLED
وضعیت Shipment باید از رویدادهای شرکت حمل‌ونقل یا ثبت عملیاتی معتبر دریافت شود.
________________


5. پرداخت و بازپرداخت
5.1. پرداخت
در نسخه اول:
* یک سفارش دارای یک مبلغ نهایی پرداختی است.
* مشتری یک پرداخت برای سفارش انجام می‌دهد.
* نتیجه پرداخت به سفارش اصلی متصل می‌شود.
* اطلاعات تراکنش و شناسه مرجع پرداخت ذخیره می‌شوند.
* وضعیت پرداخت از وضعیت ارسال جدا است.
وضعیت‌های پرداخت:
* INITIATED
* PENDING
* SUCCEEDED
* FAILED
* EXPIRED
* CANCELLED
* PARTIALLY_REFUNDED
* REFUNDED
5.2. بازپرداخت جزئی
به دلیل امکان ارسال چندمرسوله‌ای، سیستم باید بازپرداخت جزئی را پشتیبانی کند.
نمونه:
* یک قلم سفارش قابل تأمین نیست.
* دو قلم دیگر ارسال شده‌اند.
* مشتری هزینه قلم لغوشده را دریافت می‌کند.
* سفارش اصلی همچنان فعال می‌ماند.
بازپرداخت باید به Order Item یا بخش مالی مشخصی از سفارش قابل ارجاع باشد.
قواعد:
1. مجموع بازپرداخت نباید از مبلغ قابل بازپرداخت بیشتر شود.
2. هر بازپرداخت باید دلیل و مرجع مالی داشته باشد.
3. وضعیت بازپرداخت باید مستقل از وضعیت مرسوله باشد.
4. تاریخچه پرداخت و بازپرداخت قابل حذف یا بازنویسی نباشد.
5. اگر درگاه پرداخت امکان بازپرداخت خودکار نداشته باشد، فرآیند دستی کنترل‌شده لازم است.
________________


6. کنترل موجودی پیش از تأیید سفارش
پس از پرداخت موفق یا در فرآیند تأیید سفارش، سیستم باید اعتبار تأمین هر قلم را کنترل کند.
Owned Inventory
* موجودی قابل فروش بررسی شود.
* موجودی برای سفارش رزرو شود.
* در صورت تأیید نهایی، از موجودی قابل فروش کسر شود.
* در صورت لغو، رزرو آزاد شود.
Supplier Sourced
* موجودی یا ظرفیت تأمین‌کننده بررسی شود.
* قیمت و زمان آماده‌سازی اعتبارسنجی شوند.
* تأیید تأمین ثبت شود.
* در صورت عدم تأیید، سفارش وارد وضعیت بررسی عملیاتی شود.
قاعده مهم
نباید موجودی اعلام‌شده توسط تأمین‌کننده بدون اعتبارسنجی به‌عنوان موجودی قطعی نمایش داده شود.
اگر پس از پرداخت، یک قلم غیرقابل تأمین باشد:
* مشتری باید مطلع شود.
* گزینه‌های جایگزین یا لغو همان قلم بررسی شوند.
* مبلغ قلم لغوشده بازپرداخت شود.
* سایر اقلام قابل تأمین، در صورت موافقت مشتری و امکان عملیاتی، پردازش شوند.
________________


7. سیاست ارسال و هزینه حمل
سیستم باید هزینه حمل را به تفکیک سفارش و مرسوله پشتیبانی کند.
Shipping Calculation
هزینه ارسال می‌تواند به عوامل زیر وابسته باشد:
* وزن
* حجم
* ابعاد بسته
* مبدأ ارسال
* مقصد
* شرکت حمل
* نوع سرویس
* تعداد مرسوله
* قرارداد تأمین‌کننده
Shipping Subsidy
اگر خواب‌آور بخشی از هزینه ارسال را پرداخت کند، مبلغ یارانه باید به‌عنوان هزینه تجاری ثبت شود.
قواعد نمایش به مشتری
* هزینه ارسال پیش از پرداخت شفاف باشد.
* اگر هزینه نهایی مرسوله‌ها از قبل قابل تعیین نیست، سیاست محاسبه باید پیش از پرداخت روشن باشد.
* دریافت هزینه اضافی بدون تأیید مشتری مجاز نباشد.
* زمان تحویل هر مرسوله جدا نمایش داده شود.
* تغییر زمان تحویل باید اطلاع‌رسانی شود.
________________


8. مدل داده پیشنهادی
8.1. commerce
orders
* id
* order_number
* customer_id
* status
* currency
* subtotal
* discount_total
* shipping_total
* tax_total
* grand_total
* paid_total
* refunded_total
* created_at
* updated_at
order_items
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
* fulfillment_status
Snapshot برای حفظ اطلاعات تاریخی محصول و قیمت در زمان سفارش الزامی است.
payments
* id
* order_id
* payment_method
* provider
* provider_reference
* amount
* currency
* status
* paid_at
* created_at
refunds
* id
* order_id
* payment_id
* order_item_id
* amount
* reason
* status
* provider_reference
* created_at
* completed_at
8.2. fulfillment
fulfillment_orders
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
fulfillment_items
* id
* fulfillment_order_id
* order_item_id
* quantity
* status
shipments
* id
* fulfillment_order_id
* carrier
* tracking_number
* shipping_cost
* status
* shipped_at
* delivered_at
shipment_items
* id
* shipment_id
* fulfillment_item_id
* quantity
shipment_events
* id
* shipment_id
* event_code
* description
* occurred_at
* source
* raw_payload_reference
8.3. inventory
inventory_reservations
* id
* order_item_id
* variant_id
* warehouse_id
* supplier_id
* quantity
* status
* expires_at
* created_at
inventory_movements
* id
* variant_id
* warehouse_id
* movement_type
* quantity
* reference_type
* reference_id
* created_at
8.4. pricing
product_cost_records
* id
* variant_id
* supply_type
* supplier_id
* purchase_cost
* inbound_cost
* inspection_cost
* packaging_cost
* fulfillment_cost
* payment_fee_estimate
* shipping_subsidy_estimate
* expected_return_cost
* currency
* valid_from
* valid_until
* created_at
product_price_records
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
order_financial_snapshots
* id
* order_id
* order_item_id
* net_revenue
* cogs
* variable_costs
* contribution
* calculation_version
* created_at
مقادیر مالی ثبت‌شده در سفارش نباید با تغییر قیمت جاری محصول تغییر کنند.
________________


9. APIهای پیشنهادی
Customer APIs
* POST /api/v1/cart
* GET /api/v1/cart
* POST /api/v1/checkout
* GET /api/v1/orders
* GET /api/v1/orders/{id}
* GET /api/v1/orders/{id}/shipments
* POST /api/v1/orders/{id}/cancellation-requests
* POST /api/v1/order-items/{id}/return-requests
Admin APIs
* GET /api/v1/admin/orders
* GET /api/v1/admin/orders/{id}
* POST /api/v1/admin/orders/{id}/confirm
* POST /api/v1/admin/orders/{id}/split-fulfillment
* GET /api/v1/admin/fulfillments
* POST /api/v1/admin/fulfillments/{id}/confirm
* POST /api/v1/admin/fulfillments/{id}/ship
* POST /api/v1/admin/shipments/{id}/events
* POST /api/v1/admin/refunds
* GET /api/v1/admin/product-economics
Supplier APIs — Future Compatible
* GET /api/v1/supplier/fulfillments
* GET /api/v1/supplier/fulfillments/{id}
* POST /api/v1/supplier/fulfillments/{id}/accept
* POST /api/v1/supplier/fulfillments/{id}/reject
* POST /api/v1/supplier/fulfillments/{id}/ready
* POST /api/v1/supplier/fulfillments/{id}/shipment
نسخه اول می‌تواند عملیات تأمین‌کننده را از پنل داخلی مدیریت کند؛ API مستقل تأمین‌کننده الزام راه‌اندازی اولیه نیست.
________________


10. رویدادهای دامنه و قابلیت ممیزی
رویدادهای مهم:
* OrderCreated
* PaymentSucceeded
* PaymentFailed
* OrderConfirmed
* InventoryReserved
* SupplierConfirmationRequested
* SupplierConfirmationReceived
* FulfillmentCreated
* FulfillmentReady
* ShipmentCreated
* ShipmentDispatched
* ShipmentDelivered
* OrderItemCancelled
* RefundRequested
* RefundCompleted
* OrderClosed
رویدادهای مالی و سفارش باید قابل ممیزی باشند.
برای جلوگیری از ثبت تکراری عملیات، APIهای پرداخت، تأیید سفارش، رزرو موجودی و بازپرداخت باید از Idempotency پشتیبانی کنند.
برای ارتباط پایدار میان تغییر وضعیت سفارش و انتشار رویداد، استفاده از Transactional Outbox پیشنهاد می‌شود.
________________


11. قوانین امنیتی و دسترسی
* مشتری فقط سفارش‌های متعلق به خود را مشاهده کند.
* تأمین‌کننده فقط Fulfillmentهای واگذارشده به خود را ببیند.
* تأمین‌کننده نباید قیمت فروش یا سود خواب‌آور را مشاهده کند.
* نقش انبار فقط عملیات مجاز انبار را انجام دهد.
* بازپرداخت نیازمند سطح دسترسی مالی مشخص باشد.
* تغییر قیمت خرید و فروش ثبت Audit داشته باشد.
* اطلاعات پرداخت حساس نباید در Log ثبت شوند.
* دسترسی به داده‌های مشتری محدود و کنترل‌شده باشد.
________________


12. سناریوهای آزمون پذیرش
Scenario A — سفارش از سه منبع
* مشتری سه محصول می‌خرد.
* یک Order ایجاد می‌شود.
* یک پرداخت ثبت می‌شود.
* سه Fulfillment ساخته می‌شود.
* سه مرسوله با زمان‌های مستقل ثبت می‌شوند.
* مشتری همه را در یک صفحه می‌بیند.
Scenario B — عدم تأیید یک تأمین‌کننده
* پرداخت سفارش موفق است.
* یک تأمین‌کننده موجودی را تأیید نمی‌کند.
* دو قلم دیگر قابل تأمین هستند.
* قلم ناموجود لغو می‌شود.
* بازپرداخت جزئی ثبت می‌شود.
* دو قلم دیگر ارسال می‌شوند.
Scenario C — ارسال تدریجی
* بخشی از سفارش ارسال می‌شود.
* سفارش وارد PARTIALLY_SHIPPED می‌شود.
* مرسوله اول رهگیری مستقل دارد.
* مرسوله دوم بعداً ارسال می‌شود.
* سفارش پس از تکمیل تحویل‌ها بسته می‌شود.
Scenario D — مرجوعی یک قلم
* مشتری برای یک قلم درخواست مرجوعی ثبت می‌کند.
* وضعیت سایر اقلام تغییر نمی‌کند.
* نتیجه بررسی ثبت می‌شود.
* در صورت تأیید، بازپرداخت جزئی انجام می‌شود.
Scenario E — تغییر قیمت تأمین‌کننده
* قیمت معتبر اولیه در سفارش ثبت شده است.
* تأمین‌کننده قیمت جدید اعلام می‌کند.
* سفارش‌های قبلی تغییر قیمت نمی‌دهند.
* سفارش‌های جدید با قیمت معتبر جدید محاسبه می‌شوند.
* تاریخچه تغییر قیمت حفظ می‌شود.
Scenario F — اقتصاد واحد
* قیمت خرید و هزینه‌های متغیر ثبت می‌شوند.
* سود مشارکت محاسبه می‌شود.
* هزینه ارسال و مرجوعی لحاظ می‌شوند.
* محصول با سود مشارکت منفی هشدار دریافت می‌کند.
* گزارش مالی سفارش از Snapshot زمان خرید استفاده می‌کند.
________________


13. موارد خارج از محدوده نسخه اول
* تقسیم پرداخت مشتری میان چند تأمین‌کننده
* کیف پول و تسویه حساب خودکار تأمین‌کنندگان
* مارکت‌پلیس آزاد
* تجمیع هوشمند مرسوله از چند تأمین‌کننده
* اتصال به همه شرکت‌های حمل‌ونقل
* محاسبه بلادرنگ قیمت تمام‌شده از سامانه‌های خارجی
* مدیریت خودکار قراردادهای تأمین
________________


14. معیارهای پذیرش فنی
* یک Order بتواند چند Fulfillment داشته باشد.
* یک Order بتواند یک Payment اصلی داشته باشد.
* هر Fulfillment بتواند یک یا چند Shipment داشته باشد.
* هر Shipment رهگیری مستقل داشته باشد.
* وضعیت سفارش از وضعیت مرسوله‌ها تفکیک شود.
* بازپرداخت جزئی پشتیبانی شود.
* لغو یک قلم باعث لغو غیرضروری سایر اقلام نشود.
* قیمت و مشخصات سفارش به‌صورت Snapshot حفظ شوند.
* رزرو موجودی قابل کنترل و انقضا باشد.
* عملیات حساس Idempotent باشند.
* تغییرات مالی و وضعیت‌ها Audit شوند.
* سود مشارکت در سطح Variant و Order قابل محاسبه باشد.
* اطلاعات محرمانه تجاری از مشتری و تأمین‌کننده مخفی بماند.
* سفارش‌های چندمرسوله‌ای در تجربه کاربری شفاف نمایش داده شوند.
________________


15. تصمیم‌های باز برای مرحله بعد
موارد زیر هنوز تصویب نشده‌اند:
1. پرداخت آنلاین در نسخه عملیاتی با چه درگاهی انجام شود؟
2. آیا پرداخت در محل (COD) در نسخه اول لازم است؟
3. هزینه ارسال در checkout چگونه دریافت شود؟
4. سیاست ارسال رایگان بر اساس مبلغ سفارش، وزن یا کمپین باشد؟
5. بازه نگهداری رزرو موجودی پیش از پرداخت چقدر باشد؟
6. سیاست لغو سفارش پس از تأیید تأمین‌کننده چیست؟
7. مرجوعی هر گروه محصول چه شرایطی داشته باشد؟
8. آیا مشتری می‌تواند یک قلم را از سفارش چندقلمی مستقل لغو کند؟
تا زمان تصویب جداگانه، هیچ درگاه پرداخت واقعی، سرویس پیامک پولی یا قرارداد لجستیکی نباید فعال شود.
اصل نهایی: خواب‌آور برای مشتری یک فروشگاه و یک تجربه خرید است؛ در پشت صحنه، سامانه باید توانایی مدیریت چند منبع تأمین، چند فرآیند آماده‌سازی و چند مرسوله را داشته باشد.














اثر این تصمیم‌ها بر اسناد قبلی
این سند باید به‌عنوان مرجع تکمیلی اسناد قبلی استفاده شود. موارد زیر نیز باید در نسخه‌های بعدی اسناد مادر اعمال شوند:
سند
	تغییر لازم
	KHABAVAR — R-16 · SAD v2.1
	افزودن معماری Order، Fulfillment و Shipment
	KHABAVAR — R-13 · Database Architecture & Domain Design v2.0
	افزودن جداول سفارش چندمرسوله‌ای، بازپرداخت و Snapshot مالی
	KHABAVAR — R-19 · Implementation Roadmap v1.0
	افزودن مرحله مستقل Commerce Operations
	P1 Business Analysis
	ثبت سیاست ارسال و قیمت‌گذاری به‌عنوان تصمیم قطعی
	KHABAVAR — R-04 · Product Catalog v1.0
	الزام به ثبت منبع تأمین و هزینه تمام‌شده برای هر Variant