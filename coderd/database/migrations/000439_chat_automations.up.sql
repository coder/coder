CREATE TABLE chat_automations (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,
    description         TEXT        NOT NULL DEFAULT '',
    icon                TEXT        NOT NULL DEFAULT '',
    trigger_type        TEXT        NOT NULL CHECK (trigger_type IN ('webhook', 'cron')),
    webhook_secret      TEXT,
    cron_schedule       TEXT,
    model_config_id     UUID        NOT NULL REFERENCES chat_model_configs(id),
    system_prompt       TEXT        NOT NULL DEFAULT '',
    prompt_template     TEXT        NOT NULL,
    enabled             BOOLEAN     NOT NULL DEFAULT TRUE,
    max_concurrent_runs INT         NOT NULL DEFAULT 3,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chat_automations_cron_needs_schedule
        CHECK (trigger_type != 'cron' OR cron_schedule IS NOT NULL),
    CONSTRAINT chat_automations_webhook_needs_secret
        CHECK (trigger_type != 'webhook' OR webhook_secret IS NOT NULL),
    CONSTRAINT chat_automations_name_owner_unique
        UNIQUE (owner_id, name)
);

CREATE INDEX idx_chat_automations_owner ON chat_automations(owner_id);
CREATE INDEX idx_chat_automations_enabled_cron ON chat_automations(trigger_type) WHERE enabled = TRUE AND trigger_type = 'cron';

CREATE TABLE chat_automation_runs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    automation_id   UUID        NOT NULL REFERENCES chat_automations(id) ON DELETE CASCADE,
    chat_id         UUID        REFERENCES chats(id) ON DELETE SET NULL,
    trigger_payload JSONB       NOT NULL DEFAULT '{}',
    rendered_prompt TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    error           TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_automation_runs_automation ON chat_automation_runs(automation_id, created_at DESC);
CREATE INDEX idx_chat_automation_runs_active ON chat_automation_runs(automation_id) WHERE status IN ('pending', 'running');

ALTER TABLE chats ADD COLUMN automation_id UUID REFERENCES chat_automations(id) ON DELETE SET NULL;
CREATE INDEX idx_chats_automation ON chats(automation_id) WHERE automation_id IS NOT NULL;
