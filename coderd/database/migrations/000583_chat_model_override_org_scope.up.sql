ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_organization_id_id_key
    UNIQUE (organization_id, id);

CREATE TABLE chat_organization_model_overrides (
    id uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context text NOT NULL,
    model_config_id uuid NOT NULL,
    reasoning_effort text,
    CONSTRAINT chat_organization_model_overrides_context_check
        CHECK (context IN ('general', 'explore', 'title_generation', 'compaction', 'advisor')),
    CONSTRAINT chat_organization_model_overrides_organization_id_context_key
        UNIQUE (organization_id, context),
    CONSTRAINT chat_organization_model_overrides_organization_model_config_fkey
        FOREIGN KEY (organization_id, model_config_id)
        REFERENCES chat_model_configs (organization_id, id)
);

CREATE TABLE chat_user_model_overrides (
    id uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    context text NOT NULL,
    mode text NOT NULL,
    model_config_id uuid,
    reasoning_effort text,
    CONSTRAINT chat_user_model_overrides_context_check
        CHECK (context IN ('root', 'general', 'explore')),
    CONSTRAINT chat_user_model_overrides_mode_check
        CHECK (mode IN ('model', 'chat_default', 'deployment_default')),
    CONSTRAINT chat_user_model_overrides_model_requires_config_check
        CHECK ((mode = 'model') = (model_config_id IS NOT NULL)),
    CONSTRAINT chat_user_model_overrides_user_organization_context_key
        UNIQUE (user_id, organization_id, context),
    CONSTRAINT chat_user_model_overrides_organization_model_config_fkey
        FOREIGN KEY (organization_id, model_config_id)
        REFERENCES chat_model_configs (organization_id, id)
);

-- Legacy serialized overrides are dropped rather than migrated; admins and
-- users re-select models in the organization-scoped settings. Stale advisor
-- model fields inside agents_advisor_config are left as-is: reads ignore
-- unknown JSON keys and the next settings write rewrites the blob.
DELETE FROM site_configs
WHERE key IN (
    'agents_chat_general_model_override',
    'agents_chat_explore_model_override',
    'agents_chat_title_generation_model_override',
    'agents_chat_compaction_model_override'
);

DELETE FROM user_configs
WHERE key LIKE 'chat\_personal\_model\_override:%';
