BEGIN;

ALTER TABLE templates DROP COLUMN user_acl;
ALTER TABLE templates DROP COLUMN is_private;
DROP TYPE template_role;

COMMIT;
