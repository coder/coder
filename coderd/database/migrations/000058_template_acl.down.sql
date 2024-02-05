DROP TABLE group_members;
DROP TABLE groups;
ALTER TABLE templates DROP COLUMN group_acl;
ALTER TABLE templates DROP COLUMN user_acl;
