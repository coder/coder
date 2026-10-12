ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:share';

ALTER TABLE chat_projects
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE chat_projects
    ADD CONSTRAINT chat_projects_user_acl_is_object
        CHECK (jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_projects_group_acl_is_object
        CHECK (jsonb_typeof(group_acl) = 'object');

COMMENT ON COLUMN chat_projects.user_acl IS 'Users the project is shared with, keyed by user ID.';
COMMENT ON COLUMN chat_projects.group_acl IS 'Groups the project is shared with, keyed by group ID. The organization ID is the Everyone group.';

CREATE INDEX idx_chat_projects_user_acl ON chat_projects USING gin (user_acl);
CREATE INDEX idx_chat_projects_group_acl ON chat_projects USING gin (group_acl);

ALTER TABLE chat_projects ADD COLUMN deleted boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN chat_projects.deleted IS 'Irreversible. The project''s archived chats follow chat retention (kept forever when retention is 0); dbpurge deletes the row, and its memories by cascade, once none are left.';

-- Sub-chats belong to their root's project, so project_id alone records
-- membership.
UPDATE chats
SET project_id = root.project_id
FROM chats root
WHERE chats.root_chat_id = root.id
    AND root.project_id IS NOT NULL
    AND chats.project_id IS NULL;

COMMENT ON COLUMN chats.project_id IS 'Project the chat belongs to. Sub-chats carry their root chat''s project.';
