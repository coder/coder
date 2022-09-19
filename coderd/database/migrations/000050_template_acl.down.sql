BEGIN;

ALTER TABLE templates DROP COLUMN user_acl;
DROP TYPE template_role;

COMMIT;
