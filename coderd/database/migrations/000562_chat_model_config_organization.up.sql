-- Org-scope chat model configs (CODAGT-709): every config gains an
-- owning organization, backfilled to the default org. The per-org unique
-- index below is what makes at-most-one-default per org enforceable from
-- here on.

ALTER TABLE chat_model_configs
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

UPDATE chat_model_configs
SET organization_id = (SELECT id FROM organizations WHERE is_default = true LIMIT 1);

ALTER TABLE chat_model_configs ALTER COLUMN organization_id SET NOT NULL;

CREATE INDEX idx_chat_model_configs_organization_id ON chat_model_configs (organization_id);

-- Replace the deployment-wide single-default index with a per-org one.
-- The predicate is unchanged.
DROP INDEX idx_chat_model_configs_single_default;
CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs (organization_id)
    WHERE is_default = true AND deleted = false;

ALTER TABLE chat_model_configs
    ADD COLUMN group_acl jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN user_acl jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD CONSTRAINT chat_model_configs_group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object'::text),
    ADD CONSTRAINT chat_model_configs_user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object'::text);

-- Seed every existing row with the everyone-in-org read entry. The
-- Everyone group of an organization always has the organization's own ID
-- (see 000058), so the group_acl key is the org ID itself.
UPDATE chat_model_configs
SET group_acl = jsonb_build_object(
    organization_id::text,
    jsonb_build_object('permissions', jsonb_build_array('read'))
);
