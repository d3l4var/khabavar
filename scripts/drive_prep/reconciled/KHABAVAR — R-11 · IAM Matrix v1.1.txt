KHABAVAR — IAM Matrix v1.1
Document ID: KH-IAM-MATRIX-001 (سابقاً KH-IAM-001 — رفع تصادم با CIAM با تصمیم مالک 2026-11-25)
Version: 1.1 — مصوب (Owner Approval: 2026-11-24؛ بازبینی 2026-11-25 — بازگشت SUPPLIER_OPERATOR و تغییر شناسه)
Date: 2026-11-24
مبنای نسخه ۱: SAD v2.0 مصوب (§۹ پنل مدیریت و اصل Least Privilege)، CIAM v1.0 (OTP و نشست سرویس‌محور)، Seed 001_seed_baseline.sql (مبدأ واقعی ماتریس — نقش‌ها و مجوزها فقط از Seed خوانده می‌شوند، نه از فرض کد)، OpenAPI KH-API-001، Migrations 000001 و 000010، سجل تطبیق KH-INDEX-001 v1.1 §۷
هدف این نسخه: مستندسازی مصوب چرخه دسترسی پلتفرم — نقش‌های مدیریتی، مجوزهای ریزدانه، ماتریس تخصیص، سیاست OTP و نشست، و کلاینت‌های OIDC چهار سرویس؛ مرجع یکتا برای پیاده‌سازی RBAC پنل مدیریت و محدودسازی scope سرویس‌ها.

۱. اصول کنترل دسترسی
* دو لایه مکمل: دسترسی مشتریان با Scope محور (هر سرویس فقط scopeهای مختص خودش را می‌خواهد) و دسترسی پرسنل با نقش‌محوری ریزدانه (هر عملیات پنل به یک permission مشخص گره خورده است).
* اصل Least Privilege: هر نقش فقط حداقل مجوزهای لازم را دارد؛ نقش واحد «ادمین کلی» برای کارهای روزمره وجود ندارد و SUPER_ADMIN صرفاً برای مدیریت کاربران و تنظیمات است، نه عملیات روزانه.
* مجوزها منبع واحد دارند: جدول permissions و role_permissions در Seed؛ کد سمت سرور فقط نام permission را چک می‌کند و هرگز نقش را هاردکد نمی‌کند — افزودن نقش یا تغییر تخصیص بدون تغییر کد ممکن است.
* تلاش عملیاتی فراتر از مجوز با 403 پاس داده می‌شود و در Audit Log ثبت می‌گردد؛ هیچ مسیر پنلی بدون چک مجوز منتشر نمی‌شود و این قاعده در Test Strategy §۴ آزمون دارد.

۲. نقش‌های مدیریتی (۱۰ نقش مصوب)
Table: کد نقش | نام | شرح مصوب
SUPER_ADMIN	مدیر ارشد	دسترسی کامل شامل مدیریت کاربران و تنظیمات
STORE_MANAGER	مدیر فروشگاه	مدیریت کاتالوگ، قیمت و چرخه انتشار
CONTENT_MANAGER	مدیر محتوا	مدیریت مجله و گردش کار تحریریه
EDITOR	ویراستار	ویرایش مقاله‌ها و آماده‌سازی برای بازبینی
CONTENT_REVIEWER	بازبین محتوا	بازبینی محتوایی و سلامت محتوا پیش از انتشار
ORDER_OPERATOR	اپراتور سفارش	مدیریت سفارش‌ها، Fulfillment و مرسوله‌ها
INVENTORY_MANAGER	مدیر موجودی	انبارها، موجودی و رزروها
SUPPORT_OPERATOR	اپراتور پشتیبانی	پیگیری سفارش مشتریان و درخواست‌های لغو/مرجوعی
SUPPLIER_OPERATOR	اپراتور تأمین	ارزیابی تأمین‌کنندگان و تغییر وضعیت — بازگردانده از CIAM §۶ (تصمیم 2026-11-25)
FINANCE	مالی	بازپرداخت، اقتصاد واحد و گزارش‌های مالی

۳. مجوزهای ریزدانه (۲۴ مجوز — نام‌گذاری module.entity.action)
Table: کد مجوز | شرح
catalog.product.view	مشاهده محصولات و Variantها
catalog.product.edit	ایجاد/ویرایش محصول و Variant
catalog.product.publish	تأیید QC-05 و انتشار محصول
catalog.attribute.manage	مدیریت ویژگی‌ها و دسته‌بندی‌ها
supplier.manage	ثبت و تغییر وضعیت تأمین‌کننده
supplier.evaluate	ثبت ارزیابی وزنی تأمین‌کننده
inventory.manage	انبار، موجودی و حرکات
inventory.adjust	اصلاح موجودی با حرکت معکوس/ADJUSTMENT
pricing.cost.manage	ثبت هزینه تمام‌شده
pricing.price.approve	تأیید قیمت فروش و سود مشارکت
order.view	مشاهده سفارش‌ها
order.manage	تأیید، تفکیک Fulfillment، ثبت مرسوله
order.cancel.approve	تأیید لغو سفارش/قلم
order.refund.approve	تأیید و اجرای بازپرداخت (نقش FINANCE)
content.article.edit	ایجاد/ویرایش مقاله
content.article.review	بازبینی محتوایی و سلامت محتوا
content.article.publish	انتشار/لغو انتشار مقاله
content.relation.manage	ثبت رابطه مقاله-محصول با دلیل
identity.user.manage	مدیریت کاربران، نقش‌ها و مجوزها
identity.customer.view	مشاهده مشتریان و آدرس‌ها
reports.view	داشبوردها و گزارش‌ها
reports.financial.view	گزارش‌های مالی و اقتصاد واحد
audit.read	مشاهده Audit Log
settings.manage	تنظیمات سیستم

۴. ماتریس نقش → مجوز (تخصیص مصوب Seed)
Table: نقش | مجوزهای تخصیص‌یافته
SUPER_ADMIN	همه ۲۴ مجوز (بدون استثنا)
STORE_MANAGER	catalog.product.view، catalog.product.edit، catalog.product.publish، catalog.attribute.manage، pricing.cost.manage، pricing.price.approve، order.view، reports.view
CONTENT_MANAGER	content.article.edit، content.article.review، content.article.publish، content.relation.manage، catalog.product.view، reports.view
EDITOR	content.article.edit، catalog.product.view
CONTENT_REVIEWER	content.article.review، content.article.edit، catalog.product.view
ORDER_OPERATOR	order.view، order.manage، order.cancel.approve، inventory.manage، identity.customer.view، reports.view
INVENTORY_MANAGER	inventory.manage، inventory.adjust، catalog.product.view، reports.view
SUPPORT_OPERATOR	order.view، identity.customer.view
SUPPLIER_OPERATOR	supplier.manage، supplier.evaluate
FINANCE	order.refund.approve، pricing.cost.manage، pricing.price.approve، reports.financial.view، audit.read، reports.view

نکات داوری‌پذیر ماتریس: بازپرداخت انحصاراً با FINANCE است و هیچ نقش دیگری order.refund.approve ندارد؛ تأیید قیمت فروش (pricing.price.approve) به STORE_MANAGER و FINANCE مشترک است تا قفل تجاری پیشنهاد/تأیید به نقش‌های مجاز باقی بماند؛ SUPPORT_OPERATOR فقط خواننده سفارش است و لغو با او نیست (تأیید لغو با ORDER_OPERATOR)؛ ویراستار و بازبین هر دو می‌توانند ویرایش کنند اما انتشار فقط با CONTENT_MANAGER است و بازبینی خودِ مقاله‌های ویراستارِ دیگر طبق گردش کار SAD §۷.۲ انجام می‌شود؛ supplier.manage و supplier.evaluate با نقش اختصاصی SUPPLIER_OPERATOR انجام می‌شود که از CIAM §۶ بازگردانده شده و ماتریس اکنون ۱۰ نقش دارد (تصمیم مالک 2026-11-25).

۵. سیاست OTP و نشست (مصوب مرحله ۳)
* کد OTP شش‌رقمی است، عمر هر کد ۱۲۰ ثانیه، حداکثر ۵ ارسال در ساعت به هر شماره، و پس از تلاش‌های ناموفق پیاپی شماره قفل می‌شود؛ کد خام هرگز ذخیره نمی‌شود (فقط هش) و در پاسخ‌ها یا لاگ‌ها ظاهر نمی‌گردد.
* نشست به ازای هر سرویس مستقل است: ابطال نشست پنل مدیریت، نشست فروشگاه همان کاربر را نمی‌بندد — جداسازی ریسک بین محصولات.
* Access Token عمر کوتاه دارد، Refresh Token در هر استفاده چرخش می‌شود و ابطال سمت سرور فعال است (جدول نشست‌ها در Migrations 000001)؛ «خروج از همه دستگاه‌ها» با حذف نشست‌های همان سرویس پیاده می‌شود.
* Redirect URI هر کلاینت فقط از whitelist Seed پذیرفته می‌شود؛ جریان ورود Authorization Code + PKCE است و continuation_token تأیید OTP یک‌بارمصرف و عمر کوتاه دارد.

۶. کلاینت‌های OIDC چهار سرویس (Seed مصوب)
Table: client_id | سرویس | Redirect URI | Scopeها
khabavar-main	main	https://khabavar.ir/auth/callback	openid، profile، customer.basic
khabavar-shop	shop	https://shop.khabavar.ir/auth/callback	openid، profile، customer.commerce
khabavar-magazine	magazine	https://magazine.khabavar.ir/auth/callback	openid، profile، customer.content
khabavar-admin	admin	https://admin.khabavar.ir/auth/callback	openid، profile، staff

قواعد Scope: هیچ سرویس مشتری scope staff را نمی‌گیرد و پنل مدیریت هرگز scopeهای مشتری را درخواست نمی‌کند؛ این جداسازی یعنی نشست فروشگاه هرگز پنل را باز نمی‌کند و برعکس. تغییر Redirect یا Scope فقط با نسخه جدید Seed و تأیید مالک انجام می‌شود.

۷. عملیات حساس و Audit Log
* عملیات این هفت دسته در Audit Log ثبت می‌شوند و دستکاری‌ناپذیرند: قیمت فروش، هزینه خرید، تأیید تأمین، تغییر موجودی، بازپرداخت، تغییر وضعیت سفارش و شرایط ارسال.
* مشاهده Audit Log فقط با audit.read است که تنها SUPER_ADMIN و FINANCE دارند؛ رکوردها فقط‌خواندنی‌اند و در پنل با فیلتر بازیکن، نه ویرایشگر.
* تغییر نقش یا مجوز کاربران پرسنلی تنها با identity.user.manage (SUPER_ADMIN) ممکن است و خودِ این تغییر هم در Audit ثبت می‌شود تا زنجیره «چه‌کسی‌چه‌کسی» بدون شکاف بماند.
