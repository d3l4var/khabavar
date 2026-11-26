-- 000008_content.up.sql
-- ماژول Content — مطابق DB v2 §۱۲ + گردش کار تحریریه (SAD §۷.۲) + ADR-CONTENT-001

BEGIN;

CREATE TABLE IF NOT EXISTS topics (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    parent_id  UUID REFERENCES topics(id),
    sort_order INT NOT NULL DEFAULT 0,
    status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- ۱۲ موضوع پایه در Seed (DB v2 §12.2 / SAD §7.1)

CREATE TABLE IF NOT EXISTS articles (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title             VARCHAR(300) NOT NULL,
    slug              VARCHAR(255) NOT NULL UNIQUE,  -- §۱۵
    summary           TEXT,
    body              TEXT NOT NULL,
    language          VARCHAR(10) NOT NULL DEFAULT 'fa-IR',
    source_type       TEXT NOT NULL DEFAULT 'ORIGINAL'
                      CHECK (source_type IN ('ORIGINAL','TRANSLATION','CURATED')),  -- SAD §7.3
    source_url        TEXT,
    parent_article_id UUID REFERENCES articles(id),  -- نسخه‌های زبانی → مقاله مبدأ (SAD §7.2 ترجمه)
    author_id         UUID REFERENCES admin_users(id),
    editor_id         UUID REFERENCES admin_users(id),
    status            TEXT NOT NULL DEFAULT 'DRAFT'
                      CHECK (status IN ('DRAFT','IN_REVIEW','APPROVED','SCHEDULED',
                                        'PUBLISHED','UNPUBLISHED','ARCHIVED')),  -- SAD §7.2
    published_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_articles_status ON articles (status, published_at DESC);  -- §۱۵

-- رابطه مقاله-موضوع (چندبه‌چند)
CREATE TABLE IF NOT EXISTS article_topics (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    topic_id   UUID NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, topic_id)
);

-- منابع علمی مقاله — ADR-CONTENT-001: مقاله بدون منبع معتبر قابل انتشار نیست
-- (کنترل «انتشار فقط با ≥۱ منبع» در گردش کار لایه کاربرد)
CREATE TABLE IF NOT EXISTS article_sources (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id  UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('JOURNAL','BOOK','GUIDELINE','EXPERT','OTHER')),
    citation    TEXT NOT NULL,
    url         TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_article_sources ON article_sources (article_id);

-- رابطه چندبه‌چند مقاله-محصول (§12.3) با دلیل ارتباط (PRD MA-5)
CREATE TABLE IF NOT EXISTS product_article_relations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    UUID NOT NULL REFERENCES products(id),
    article_id    UUID NOT NULL REFERENCES articles(id),
    relation_type TEXT NOT NULL CHECK (relation_type IN
                  ('RELATED','EDUCATIONAL','PRODUCT_GUIDE','COMPARISON')),   -- §12.3 عیناً
    relation_reason TEXT NOT NULL,      -- MA-5: «دلیل ارتباط» ثبت‌شده، نه تبلیغ صرف
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, article_id, relation_type)
);
CREATE INDEX IF NOT EXISTS ix_par_article ON product_article_relations (article_id);

CREATE TRIGGER trg_articles_updated BEFORE UPDATE ON articles FOR EACH ROW EXECUTE FUNCTION khabavar_set_updated_at();

COMMIT;
