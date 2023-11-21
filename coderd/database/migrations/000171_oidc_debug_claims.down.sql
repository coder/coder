BEGIN;

ALTER TABLE user_links DROP COLUMN debug_context jsonb;

COMMIT;
