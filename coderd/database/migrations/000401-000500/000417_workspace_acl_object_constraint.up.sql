-- Add constraints that reject 'null'::jsonb for group and user ACLs
-- because they would break the new workspace_expanded view.

UPDATE workspaces SET group_acl = '{}'::jsonb WHERE group_acl = 'null'::jsonb;
UPDATE workspaces SET user_acl = '{}'::jsonb WHERE user_acl = 'null'::jsonb;

ALTER TABLE workspaces
    ADD CONSTRAINT group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object'),
    ADD CONSTRAINT user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object');
