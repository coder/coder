BEGIN;


DROP TABLE group_members;
DROP TABLE groups;
DROP TYPE template_role;
ALTER TABLE templates DROP COLUMN group_acl;
ALTER TABLE templates DROP COLUMN user_acl;

COMMIT;
