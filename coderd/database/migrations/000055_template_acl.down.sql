BEGIN;

ALTER TABLE templates DROP COLUMN user_acl;
ALTER TABLE templates DROP COLUMN is_private;
DROP TYPE template_role;

DROP TABLE groups;
DROP TABLE group_users;

COMMIT;
