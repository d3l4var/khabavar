-- 000013_relation_reason_check.up.sql
-- MA-5 defence in depth: a product↔article relation must carry a real reason, not an
-- empty string. The service already refuses a blank reason (ErrRelationReasonReq), but
-- `relation_reason TEXT NOT NULL` accepts '' — so a direct write could bypass the rule
-- and turn the field into a formality. Same pattern as chk_products_publish_gates.

BEGIN;

ALTER TABLE product_article_relations
    ADD CONSTRAINT chk_relation_reason_not_blank
    CHECK (btrim(relation_reason) <> '');

COMMIT;
