-- 000013_relation_reason_check.down.sql
-- Forward-only policy (DEP §۶): emergency use only. Dropping the constraint re-opens
-- the MA-5 bypass.

BEGIN;

ALTER TABLE product_article_relations
    DROP CONSTRAINT IF EXISTS chk_relation_reason_not_blank;

COMMIT;
