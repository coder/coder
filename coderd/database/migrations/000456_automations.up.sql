-- Chat automations bridge external events (webhooks, cron schedules) to
-- Coder chats. A chat automation defines *what* to say, *which* model
-- and tools to use, and *how fast* it is allowed to create or continue
-- chats.

CREATE TYPE chat_automation_status AS ENUM ('disabled', 'preview', 'active');
CREATE TYPE chat_automation_trigger_type AS ENUM ('webhook', 'cron');
CREATE TYPE chat_automation_event_status AS ENUM ('filtered', 'preview', 'created', 'continued', 'rate_limited', 'error');

CREATE TABLE chat_automations (
    id uuid NOT NULL,
    -- The user on whose behalf chats are created. All RBAC checks and
    -- chat ownership are scoped to this user.
    owner_id uuid NOT NULL,
    -- Organization scope for RBAC. Combined with owner_id and name to
    -- form a unique constraint so automations are namespaced per user
    -- per org.
    organization_id uuid NOT NULL,
    -- Human-readable identifier. Unique within (owner_id, organization_id).
    name text NOT NULL,
    -- Optional long-form description shown in the UI.
    description text NOT NULL DEFAULT '',
    -- The user-role message injected into every chat this automation
    -- creates. This is the core prompt that tells the LLM what to do.
    instructions text NOT NULL DEFAULT '',
    -- Optional model configuration override. When NULL the deployment
    -- default is used. SET NULL on delete so automations survive config
    -- changes gracefully.
    model_config_id uuid,
    -- MCP servers to attach to chats created by this automation.
    -- Stored as an array of UUIDs rather than a join table because
    -- the set is small and always read/written atomically.
    mcp_server_ids uuid[] NOT NULL DEFAULT '{}',
    -- Tool allowlist. Empty means all tools available to the model
    -- config are permitted.
    allowed_tools text[] NOT NULL DEFAULT '{}',
    -- Lifecycle state:
    --   disabled — trigger events are silently dropped.
    --   preview  — events are logged but no chat is created (dry-run).
    --   active   — events create or continue chats.
    status chat_automation_status NOT NULL DEFAULT 'disabled',
    -- Maximum number of *new* chats this automation may create in a
    -- rolling one-hour window. Prevents runaway webhook storms from
    -- flooding the system. Approximate under concurrency; the
    -- check-then-insert is not serialized, so brief bursts may
    -- slightly exceed the cap.
    max_chat_creates_per_hour integer NOT NULL DEFAULT 10,
    -- Maximum total messages (creates + continues) this automation may
    -- send in a rolling one-hour window. A second, broader throttle
    -- that catches high-frequency continuation patterns. Same
    -- approximate-under-concurrency caveat as above.
    max_messages_per_hour integer NOT NULL DEFAULT 60,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (model_config_id) REFERENCES chat_model_configs(id) ON DELETE SET NULL,
    CONSTRAINT chat_automations_max_chat_creates_per_hour_check CHECK (max_chat_creates_per_hour > 0),
    CONSTRAINT chat_automations_max_messages_per_hour_check CHECK (max_messages_per_hour > 0)
);

CREATE INDEX idx_chat_automations_owner_id ON chat_automations (owner_id);
CREATE INDEX idx_chat_automations_organization_id ON chat_automations (organization_id);

-- Enforces that automation names are unique per user per org so they
-- can be referenced unambiguously in CLI/API calls.
CREATE UNIQUE INDEX idx_chat_automations_owner_org_name ON chat_automations (owner_id, organization_id, name);

-- Triggers define *how* an automation is invoked. Each automation can
-- have multiple triggers (e.g. one webhook + one cron schedule).
-- Webhook and cron triggers share the same row shape with type-specific
-- nullable columns to keep the schema simple.
CREATE TABLE chat_automation_triggers (
    id uuid NOT NULL,
    -- Parent automation. CASCADE delete ensures orphan triggers are
    -- cleaned up when an automation is removed.
    automation_id uuid NOT NULL,
    -- Discriminator: 'webhook' or 'cron'. Determines which nullable
    -- columns are meaningful.
    type chat_automation_trigger_type NOT NULL,
    -- HMAC-SHA256 shared secret for webhook signature verification
    -- (X-Hub-Signature-256 header). NULL for cron triggers.
    webhook_secret text,
    -- Identifier of the dbcrypt key used to encrypt webhook_secret.
    -- NULL means the secret is not yet encrypted. When dbcrypt is
    -- enabled, this references the active key digest used for
    -- AES-256-GCM encryption.
    webhook_secret_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    -- Standard 5-field cron expression (minute hour dom month dow),
    -- with optional CRON_TZ= prefix. NULL for webhook triggers.
    cron_schedule text,
    -- Timestamp of the last successful cron fire. The scheduler
    -- computes next = cron.Next(last_triggered_at) and fires when
    -- next <= now. NULL means the trigger has never fired; the
    -- scheduler falls back to created_at as the reference time.
    -- Not used for webhook triggers.
    last_triggered_at timestamp with time zone,
    -- gjson path→value filter conditions evaluated against the
    -- incoming webhook payload. All conditions must match for the
    -- trigger to fire. NULL or empty means "match everything".
    filter jsonb,
    -- Maps chat label keys to gjson paths. When a trigger fires,
    -- labels are resolved from the payload and used to find an
    -- existing chat to continue (by label match) or set on a
    -- newly created chat. This is how automations route events
    -- to the right conversation.
    label_paths jsonb,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (automation_id) REFERENCES chat_automations(id) ON DELETE CASCADE,
    CONSTRAINT chat_automation_triggers_webhook_fields CHECK (
        type != 'webhook' OR (cron_schedule IS NULL AND last_triggered_at IS NULL)
    ),
    CONSTRAINT chat_automation_triggers_cron_fields CHECK (
        type != 'cron' OR (webhook_secret IS NULL AND webhook_secret_key_id IS NULL)
    )
);

CREATE INDEX idx_chat_automation_triggers_automation_id ON chat_automation_triggers (automation_id);

-- Every trigger invocation produces an event row regardless of outcome.
-- This table is the audit trail and the data source for rate-limit
-- window counts. Rows are append-only and expected to be purged by a
-- background job after a retention period.
CREATE TABLE chat_automation_events (
    id uuid NOT NULL,
    -- The automation that owns this event.
    automation_id uuid NOT NULL,
    -- The trigger that produced this event. SET NULL on delete so
    -- historical events survive trigger removal.
    trigger_id uuid,
    -- When the event was received (webhook delivery time or cron
    -- evaluation time). Used for rate-limit window calculations and
    -- purge cutoffs.
    received_at timestamp with time zone NOT NULL,
    -- The raw payload that was evaluated. For webhooks this is the
    -- HTTP body; for cron triggers it is a synthetic JSON envelope
    -- with schedule metadata.
    payload jsonb NOT NULL,
    -- Whether the trigger's filter conditions matched. False means
    -- the event was dropped before any chat interaction.
    filter_matched boolean NOT NULL,
    -- Labels resolved from the payload via label_paths. Stored so
    -- the event log shows exactly which labels were computed.
    resolved_labels jsonb,
    -- ID of an existing chat that was found via label matching and
    -- continued with a new message.
    matched_chat_id uuid,
    -- ID of a newly created chat (mutually exclusive with
    -- matched_chat_id in practice).
    created_chat_id uuid,
    -- Outcome of the event:
    --   filtered     — filter did not match, event dropped.
    --   preview      — automation is in preview mode, no chat action.
    --   created      — new chat was created.
    --   continued    — existing chat was continued.
    --   rate_limited — rate limit prevented chat action.
    --   error        — something went wrong (see error column).
    status chat_automation_event_status NOT NULL,
    -- Human-readable error description when status = 'error' or
    -- 'rate_limited'. NULL for successful outcomes.
    error text,
    PRIMARY KEY (id),
    FOREIGN KEY (automation_id) REFERENCES chat_automations(id) ON DELETE CASCADE,
    FOREIGN KEY (trigger_id) REFERENCES chat_automation_triggers(id) ON DELETE SET NULL,
    FOREIGN KEY (matched_chat_id) REFERENCES chats(id) ON DELETE SET NULL,
    FOREIGN KEY (created_chat_id) REFERENCES chats(id) ON DELETE SET NULL,
    CONSTRAINT chat_automation_events_chat_exclusivity CHECK (
        matched_chat_id IS NULL OR created_chat_id IS NULL
    ),
    -- Enforce that 'created' events have a created_chat_id and
    -- 'continued' events have a matched_chat_id.
    CONSTRAINT chat_automation_events_status_chat_consistency CHECK (
        (status != 'created' OR created_chat_id IS NOT NULL) AND
        (status != 'continued' OR matched_chat_id IS NOT NULL)
    )
);

-- Composite index for listing events per automation in reverse
-- chronological order (the primary UI query pattern).
CREATE INDEX idx_chat_automation_events_automation_id_received_at ON chat_automation_events (automation_id, received_at DESC);

-- Standalone index on received_at for the purge job, which deletes
-- events older than the retention period across all automations.
CREATE INDEX idx_chat_automation_events_received_at ON chat_automation_events (received_at);

-- Partial index for rate-limit window count queries, which filter
-- by automation_id and status IN ('created', 'continued').
CREATE INDEX idx_chat_automation_events_rate_limit
  ON chat_automation_events (automation_id, received_at)
  WHERE status IN ('created', 'continued');

-- Link chats back to the automation that created them. SET NULL on
-- delete so chats survive if the automation is removed. Indexed for
-- lookup queries that list chats spawned by a given automation.
ALTER TABLE chats ADD COLUMN automation_id uuid REFERENCES chat_automations(id) ON DELETE SET NULL;

CREATE INDEX idx_chats_automation_id ON chats (automation_id);

-- Add API key scope values for the new chat_automation resource type.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:*';
