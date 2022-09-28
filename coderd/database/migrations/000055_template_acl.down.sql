BEGIN;

ALTER TABLE templates DROP COLUMN user_acl;
ALTER TABLE templates DROP COLUMN group_acl;
DROP TYPE template_role;

DROP TABLE groups;
DROP TABLE group_members;

COMMIT;
