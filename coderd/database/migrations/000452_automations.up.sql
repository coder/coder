CREATE TABLE automations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    instructions text NOT NULL DEFAULT '',
    model_config_id uuid,
    mcp_server_ids uuid[] NOT NULL DEFAULT '{}',
    allowed_tools text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'disabled',
    max_chat_creates_per_hour integer NOT NULL DEFAULT 10,
    max_messages_per_hour integer NOT NULL DEFAULT 60,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (model_config_id) REFERENCES chat_model_configs(id) ON DELETE SET NULL,
    CONSTRAINT automations_status_check CHECK (status IN ('disabled', 'preview', 'active')),
    CONSTRAINT automations_max_chat_creates_per_hour_check CHECK (max_chat_creates_per_hour > 0),
    CONSTRAINT automations_max_messages_per_hour_check CHECK (max_messages_per_hour > 0)
);

COMMENT ON COLUMN automations.instructions IS 'User message sent to the chat when the automation triggers.';

CREATE INDEX idx_automations_owner_id ON automations (owner_id);
CREATE INDEX idx_automations_organization_id ON automations (organization_id);

CREATE TABLE automation_triggers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    automation_id uuid NOT NULL,
    type text NOT NULL,
    webhook_secret text,
    webhook_secret_key_id text,
    cron_schedule text,
    filter jsonb,
    label_paths jsonb,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    FOREIGN KEY (automation_id) REFERENCES automations(id) ON DELETE CASCADE,
    CONSTRAINT automation_triggers_type_check CHECK (type IN ('webhook', 'cron'))
);

COMMENT ON COLUMN automation_triggers.webhook_secret_key_id IS 'The ID of the key used to encrypt the webhook secret. If NULL, the secret is not encrypted.';
COMMENT ON COLUMN automation_triggers.filter IS 'gjson filter conditions for webhook triggers. NULL means match everything.';
COMMENT ON COLUMN automation_triggers.label_paths IS 'Map of chat label keys to gjson paths for extracting values from webhook payloads.';

CREATE INDEX idx_automation_triggers_automation_id ON automation_triggers (automation_id);

CREATE TABLE automation_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    automation_id uuid NOT NULL,
    trigger_id uuid,
    received_at timestamp with time zone NOT NULL DEFAULT now(),
    payload jsonb NOT NULL,
    filter_matched boolean NOT NULL,
    resolved_labels jsonb,
    matched_chat_id uuid,
    created_chat_id uuid,
    status text NOT NULL,
    error text,
    PRIMARY KEY (id),
    FOREIGN KEY (automation_id) REFERENCES automations(id) ON DELETE CASCADE,
    FOREIGN KEY (trigger_id) REFERENCES automation_triggers(id) ON DELETE SET NULL,
    CONSTRAINT automation_events_status_check CHECK (status IN ('filtered', 'preview', 'created', 'continued', 'rate_limited', 'error'))
);

CREATE INDEX idx_automation_events_automation_id_received_at ON automation_events (automation_id, received_at DESC);

ALTER TABLE chats ADD COLUMN automation_id uuid REFERENCES automations(id) ON DELETE SET NULL;
