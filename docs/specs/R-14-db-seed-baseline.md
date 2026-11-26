-- 001_seed_baseline.sql
-- Seed Data طبق DB v2 §۱۷ — فقط داده‌های سیستمی؛ هیچ تأمین‌کننده/موجودی/قیمت فرضی وارد نمی‌شود.
-- اجرا: پس از همه Migrationها، به ازای هر محیط (Development/Staging/Production) یک‌بار.

BEGIN;

-- ── نقش‌های مدیریتی (SAD v1 §۹ پنل مدیریت + FINANCE از Commerce §۱۱ + SUPPLIER_OPERATOR بازگردانده از CIAM §۶ — تصمیم مالک 2026-11-25) ──
INSERT INTO roles (code, name, description) VALUES
    ('SUPER_ADMIN',        'مدیر ارشد',           'دسترسی کامل شامل مدیریت کاربران و تنظیمات'),
    ('STORE_MANAGER',      'مدیر فروشگاه',        'مدیریت کاتالوگ، قیمت و چرخه انتشار'),
    ('CONTENT_MANAGER',    'مدیر محتوا',          'مدیریت مجله و گردش کار تحریریه'),
    ('EDITOR',             'ویراستار',            'ویرایش مقاله‌ها و آماده‌سازی برای بازبینی'),
    ('CONTENT_REVIEWER',   'بازبین محتوا',        'بازبینی محتوایی و سلامت محتوا پیش از انتشار'),
    ('ORDER_OPERATOR',     'اپراتور سفارش',       'مدیریت سفارش‌ها، Fulfillment و مرسوله‌ها'),
    ('INVENTORY_MANAGER',  'مدیر موجودی',         'انبارها، موجودی و رزروها'),
    ('SUPPORT_OPERATOR',   'اپراتور پشتیبانی',    'پیگیری سفارش مشتریان و درخواست‌های لغو/مرجوعی'),
    ('SUPPLIER_OPERATOR',  'اپراتور تأمین',       'ارزیابی تأمین‌کنندگان و تغییر وضعیت آن‌ها — بازگردانده از CIAM §۶ (تصمیم مالک 2026-11-25)'),
    ('FINANCE',            'مالی',                'بازپرداخت، اقتصاد واحد و گزارش‌های مالی')
ON CONFLICT (code) DO NOTHING;

-- ── مجوزها (نام‌گذاری: module.entity.action) ──
INSERT INTO permissions (code, resource, action, description) VALUES
    ('catalog.product.view',      'catalog', 'view',      'مشاهده محصولات و Variantها'),
    ('catalog.product.edit',      'catalog', 'edit',      'ایجاد/ویرایش محصول و Variant'),
    ('catalog.product.publish',   'catalog', 'publish',   'تأیید QC-05 و انتشار محصول'),
    ('catalog.attribute.manage',  'catalog', 'manage',    'مدیریت ویژگی‌ها و دسته‌بندی‌ها'),
    ('supplier.manage',           'supplier','manage',    'ثبت و تغییر وضعیت تأمین‌کننده'),
    ('supplier.evaluate',         'supplier','evaluate',  'ثبت ارزیابی وزنی تأمین‌کننده'),
    ('inventory.manage',          'inventory','manage',   'انبار، موجودی و حرکات'),
    ('inventory.adjust',          'inventory','adjust',   'اصلاح موجودی با حرکت معکوس/ADJUSTMENT'),
    ('pricing.cost.manage',       'pricing', 'manage',    'ثبت هزینه تمام‌شده'),
    ('pricing.price.approve',     'pricing', 'approve',   'تأیید قیمت فروش و سود مشارکت'),
    ('order.view',                'commerce','view',      'مشاهده سفارش‌ها'),
    ('order.manage',              'commerce','manage',    'تأیید، تفکیک Fulfillment، ثبت مرسوله'),
    ('order.cancel.approve',      'commerce','cancel',    'تأیید لغو سفارش/قلم'),
    ('order.refund.approve',      'commerce','refund',    'تأیید و اجرای بازپرداخت (نقش FINANCE)'),
    ('content.article.edit',      'content', 'edit',      'ایجاد/ویرایش مقاله'),
    ('content.article.review',    'content', 'review',    'بازبینی محتوایی و سلامت محتوا'),
    ('content.article.publish',   'content', 'publish',   'انتشار/لغو انتشار مقاله'),
    ('content.relation.manage',   'content', 'manage',    'ثبت رابطه مقاله-محصول با دلیل'),
    ('identity.user.manage',      'identity','manage',    'مدیریت کاربران، نقش‌ها و مجوزها'),
    ('identity.customer.view',    'identity','view',      'مشاهده مشتریان و آدرس‌ها'),
    ('reports.view',              'reports', 'view',      'داشبوردها و گزارش‌ها'),
    ('reports.financial.view',    'reports', 'financial', 'گزارش‌های مالی و اقتصاد واحد'),
    ('audit.read',                'audit',   'read',      'مشاهده Audit Log'),
    ('settings.manage',           'system',  'manage',    'تنظیمات سیستم')
ON CONFLICT (code) DO NOTHING;

-- ── تخصیص نقش → مجوز (اصل Least Privilege — SAD v1 §۹) ──
WITH r AS (SELECT id, code FROM roles),
     p AS (SELECT id, code FROM permissions)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM r JOIN p ON TRUE WHERE
    (r.code = 'SUPER_ADMIN')
 OR (r.code = 'STORE_MANAGER'     AND p.code IN ('catalog.product.view','catalog.product.edit','catalog.product.publish','catalog.attribute.manage','pricing.cost.manage','pricing.price.approve','order.view','reports.view'))
 OR (r.code = 'CONTENT_MANAGER'   AND p.code IN ('content.article.edit','content.article.review','content.article.publish','content.relation.manage','catalog.product.view','reports.view'))
 OR (r.code = 'EDITOR'            AND p.code IN ('content.article.edit','catalog.product.view'))
 OR (r.code = 'CONTENT_REVIEWER'  AND p.code IN ('content.article.review','content.article.edit','catalog.product.view'))
 OR (r.code = 'ORDER_OPERATOR'    AND p.code IN ('order.view','order.manage','order.cancel.approve','inventory.manage','identity.customer.view','reports.view'))
 OR (r.code = 'INVENTORY_MANAGER' AND p.code IN ('inventory.manage','inventory.adjust','catalog.product.view','reports.view'))
 OR (r.code = 'SUPPORT_OPERATOR'  AND p.code IN ('order.view','identity.customer.view'))
 OR (r.code = 'SUPPLIER_OPERATOR' AND p.code IN ('supplier.manage','supplier.evaluate'))
 OR (r.code = 'FINANCE'           AND p.code IN ('order.refund.approve','pricing.cost.manage','pricing.price.approve','reports.financial.view','audit.read','reports.view'))
ON CONFLICT DO NOTHING;

-- ── موضوعات پایه مجله — ۱۱ موضوع مستند در DB v2 §12.2 ──
-- توجه: SAD §۷.۱ «۱۲ دسته پایه» گفته است؛ سند DB فقط ۱۱ مورد را نام برده —
-- موضوع دوازدهم نیازمند تصمیم مالک است و در Seed فعلی لحاظ نشده (بدون ابداع).
INSERT INTO topics (name, slug, sort_order) VALUES
    ('علوم خواب',            'sleep-science',       1),
    ('بی‌خوابی',              'insomnia',            2),
    ('تغذیه',                'nutrition',           3),
    ('دارو',                 'medication',          4),
    ('نوشیدنی‌ها',            'drinks',              5),
    ('روان‌شناسی',            'psychology',          6),
    ('متابولیسم',            'metabolism',          7),
    ('سلامت جسمی',           'physical-health',     8),
    ('سلامت روانی',          'mental-health',       9),
    ('سبک زندگی',            'lifestyle',          10),
    ('فرهنگ و سنت‌ها',        'culture-traditions', 11)
ON CONFLICT (slug) DO NOTHING;

-- ── کلاینت‌های OIDC برای چهار سرویس (CIAM §۳) — مسیرهای Callback در استقرار نهایی تأیید می‌شوند ──
INSERT INTO oauth_clients (client_id, service_code, name, redirect_uris, scopes) VALUES
    ('khabavar-main',     'main',     'وب‌سایت اصلی خواب‌آور',  ARRAY['https://khabavar.ir/auth/callback'],           ARRAY['openid','profile','customer.basic']),
    ('khabavar-shop',     'shop',     'فروشگاه خواب‌آور',       ARRAY['https://shop.khabavar.ir/auth/callback'],      ARRAY['openid','profile','customer.commerce']),
    ('khabavar-magazine', 'magazine', 'مجله تخصصی خواب‌آور',    ARRAY['https://magazine.khabavar.ir/auth/callback'],  ARRAY['openid','profile','customer.content']),
    ('khabavar-admin',    'admin',    'پنل مدیریت خواب‌آور',    ARRAY['https://admin.khabavar.ir/auth/callback'],     ARRAY['openid','profile','staff'])
ON CONFLICT (client_id) DO NOTHING;

COMMIT;
