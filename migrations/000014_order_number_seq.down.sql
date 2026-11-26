-- 000014_order_number_seq.down.sql
-- Forward-only policy (DEP §۶): emergency use only. Dropping the sequence would make
-- the API unable to mint order numbers.

BEGIN;

DROP SEQUENCE IF EXISTS order_number_seq;

COMMIT;
