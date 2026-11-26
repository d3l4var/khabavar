-- 000002_catalog.up.sql
-- ماژول Catalog — مطابق DB v2 §۶ + §۲ (brands, media) + قواعد مدل A از PRD §۲ (MA-1، MA-6)

BEGIN;

-- دارایی‌های رسانه‌ای عمومی (برای تصویر دسته‌بندی و برند؛ کلید فایل در Object Storage)
CREATE TABLE IF NOT EXISTS media_assets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_type  TEXT NOT NULL DEFAULT 'IMAGE' CHECK (media_type IN ('IMAGE','VIDEO','DOCUMENT')),
    storage_key TEXT NOT NULL,
    alt_text    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- برندها (DB v2 §۲ tree: brands)
CREATE TABLE IF NOT EXISTS brands (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    logo_id    UUID REFERENCES media_assets(id),
    status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- دسته‌بندی‌ها (DB v2 §6.1 — درختی با parent_id)
CREATE TABLE IF NOT EXISTS categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES categories(id),
    name        VARCHAR(200) NOT NULL,
    slug        VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    image_id    UUID REFERENCES media_assets(id),
    sort_order  INT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_categories_parent ON categories (parent_id);   -- §۱۵

-- محصولات (DB v2 §6.2 — وضعیت‌ها عیناً از سند)
CREATE TABLE IF NOT EXISTS products (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title                VARCHAR(255) NOT NULL,
    slug                 VARCHAR(255) NOT NULL UNIQUE,                       -- §۱۵
    brand_id             UUID REFERENCES brands(id),
    short_description    TEXT,
    description          TEXT,
    manufacturer         VARCHAR(255),
    country_of_origin    VARCHAR(100),
    -- قواعد مدل A (PRD §۲ — MA-1، MA-6): بدون توجیه خواب/منبع مشخصات، انتشار ممنوع
    sleep_relevance      TEXT,          -- توجیه ارتباط واقعی محصول با خواب (MA-1)
    spec_source          TEXT,          -- منبع مشخصات فنی (MA-6)
    spec_last_reviewed_at TIMESTAMPTZ,  -- تاریخ آخرین بازبینی مشخصات (MA-6)
    status               TEXT NOT NULL DEFAULT 'DRAFT'
                         CHECK (status IN ('DRAFT','DATA_REVIEW','APPROVED','PUBLISHED','PAUSED','DISCONTINUED')),
    published_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_products_publish_gates CHECK (
        status <> 'PUBLISHED'
        OR (sleep_relevance IS NOT NULL AND sleep_relevance <> ''
            AND spec_source IS NOT NULL AND spec_source <> ''
            AND spec_last_reviewed_at IS NOT NULL)
    )
);

-- رابطه محصول-دسته‌بندی (چندبه‌چند — ERD: PRODUCTS classified CATEGORIES)
CREATE TABLE IF NOT EXISTS product_categories (
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX IF NOT EXISTS ix_product_categories_cat ON product_categories (category_id);

-- Variantهای قابل فروش (DB v2 §6.3)
CREATE TABLE IF NOT EXISTS product_variants (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku          VARCHAR(100) NOT NULL UNIQUE,                                 -- §۱۵
    barcode      VARCHAR(100),
    attributes   JSONB,                       -- فقط مشخصات انعطاف‌پذیر غیرهسته‌ای (§6.3)
    weight_grams INT CHECK (weight_grams IS NULL OR weight_grams >= 0),
    length_mm    INT CHECK (length_mm  IS NULL OR length_mm  >= 0),
    width_mm     INT CHECK (width_mm   IS NULL OR width_mm   >= 0),
    height_mm    INT CHECK (height_mm  IS NULL OR height_mm  >= 0),
    status       TEXT NOT NULL DEFAULT 'DRAFT'
                 CHECK (status IN ('DRAFT','ACTIVE','INACTIVE','DISCONTINUED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_variants_product ON product_variants (product_id);

-- ویژگی‌های ساختاریافته (DB v2 §6.4)
CREATE TABLE IF NOT EXISTS attribute_definitions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(100) NOT NULL UNIQUE,
    name          VARCHAR(200) NOT NULL,
    data_type     TEXT NOT NULL CHECK (data_type IN ('TEXT','NUMBER','BOOLEAN','OPTION')),
    unit          VARCHAR(30),                 -- برای مقادیر عددی واحد الزامی است (§6.4)
    is_filterable BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS attribute_options (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attribute_id UUID NOT NULL REFERENCES attribute_definitions(id) ON DELETE CASCADE,
    value        VARCHAR(200) NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    UNIQUE (attribute_id, value)
);
CREATE TABLE IF NOT EXISTS product_attribute_values (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id   UUID REFERENCES product_variants(id) ON DELETE CASCADE,
    attribute_id UUID NOT NULL REFERENCES attribute_definitions(id),
    option_id    UUID REFERENCES attribute_options(id),
    value_text   TEXT,
    value_number NUMERIC,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- دقیقاً یک شکل مقدار معتبر
    CHECK ((option_id IS NOT NULL)::int + (value_text IS NOT NULL)::int + (value_number IS NOT NULL)::int = 1)
);
CREATE INDEX IF NOT EXISTS ix_pav_product  ON product_attribute_values (product_id, attribute_id);
CREATE INDEX IF NOT EXISTS ix_pav_variant  ON product_attribute_values (variant_id, attribute_id);

-- تصاویر/رسانه محصول (DB v2 §6.5 — دیتابیس فقط storage_key نگه می‌دارد)
CREATE TABLE IF NOT EXISTS product_media (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id  UUID REFERENCES product_variants(id) ON DELETE CASCADE,
    media_type  TEXT NOT NULL DEFAULT 'IMAGE' CHECK (media_type IN ('IMAGE','VIDEO','DOCUMENT')),
    storage_key TEXT NOT NULL,
    alt_text    TEXT,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_product_media_product ON product_media (product_id, sort_order);

CREATE TRIGGER trg_brands_updated    BEFORE UPDATE ON brands    FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_categories_updated BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_products_updated  BEFORE UPDATE ON products  FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();
CREATE TRIGGER trg_variants_updated  BEFORE UPDATE ON product_variants FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;
