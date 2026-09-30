CREATE TABLE workspace_execution_sessions (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    owner_id UUID NOT NULL REFERENCES users(id),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id UUID NOT NULL,
    input_digest BYTEA NOT NULL CHECK (octet_length(input_digest) = 32),
    workspace_id UUID,
    workspace_owner_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN (
        'active', 'preserving', 'preserved',
        'preservation_failed', 'retained', 'deleting', 'deletion_failed', 'completed'
    )),
    disposable BOOLEAN NOT NULL DEFAULT FALSE,
    retained BOOLEAN NOT NULL DEFAULT TRUE,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    declarations JSONB NOT NULL CHECK (jsonb_typeof(declarations) = 'object'),
    delete_build_id UUID,
    next_retry_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    error TEXT NOT NULL DEFAULT '',
    acquisition_build_id UUID,
    recovery_artifact_expires_at TIMESTAMPTZ,
    CONSTRAINT workspace_execution_sessions_identity UNIQUE (id, organization_id, owner_id),
    CONSTRAINT workspace_execution_sessions_request_key UNIQUE (organization_id, actor_id, request_id),
    CHECK ((workspace_id IS NULL) = (workspace_owner_id IS NULL)),
    CHECK (state NOT IN ('active', 'preserving', 'deleting') OR workspace_id IS NOT NULL),
    CHECK (state <> 'deleting' OR (disposable AND NOT retained AND delete_build_id IS NOT NULL))
);

-- Source identities remain available after workspace and build history is purged.
COMMENT ON COLUMN workspace_execution_sessions.workspace_id IS
    'Recorded source workspace identity, intentionally independent of workspace deletion.';
COMMENT ON COLUMN workspace_execution_sessions.workspace_owner_id IS
    'Workspace owner at acquisition, checked again before automatic cleanup.';
COMMENT ON COLUMN workspace_execution_sessions.declarations IS
    'Immutable versioned output and cleanup declarations accepted before execution.';
COMMENT ON COLUMN workspace_execution_sessions.input_digest IS
    'Canonical acquisition input digest; retained with request identity to prevent replay after expiry.';

CREATE INDEX workspace_execution_sessions_workspace_id_idx
    ON workspace_execution_sessions (workspace_id);
CREATE INDEX workspace_execution_sessions_due_idx
    ON workspace_execution_sessions (next_retry_at, lease_expires_at)
    WHERE state <> 'completed';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_execution:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_execution:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_execution:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_execution:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_execution:ssh';

CREATE TABLE workspace_execution_receipts (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    owner_id UUID NOT NULL REFERENCES users(id),
    actor_id UUID NOT NULL REFERENCES users(id),
    request_id UUID NOT NULL,
    input_digest BYTEA NOT NULL CHECK (octet_length(input_digest) = 32),
    workspace_id UUID NOT NULL,
    workspace_owner_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    agent_instance_id UUID NOT NULL,
    process_id UUID NOT NULL,
    admission_revision BIGINT NOT NULL CHECK (admission_revision > 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deadline TIMESTAMPTZ,
    state TEXT NOT NULL CHECK (state IN (
        'dispatching', 'running', 'completed', 'unknown', 'not_started'
    )),
    exit_code INTEGER,
    error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (session_id, organization_id, owner_id)
        REFERENCES workspace_execution_sessions (id, organization_id, owner_id),
    CONSTRAINT workspace_execution_receipts_key UNIQUE (session_id, actor_id, request_id),
    CONSTRAINT workspace_execution_receipts_process UNIQUE (agent_instance_id, process_id),
    CHECK ((state = 'completed') = (exit_code IS NOT NULL))
);
COMMENT ON TABLE workspace_execution_receipts IS
    'Durable execution identities. Command and environment payloads are never stored for replay.';
COMMENT ON COLUMN workspace_execution_receipts.workspace_id IS
    'Source identity retained after workspace deletion or purging.';
CREATE INDEX workspace_execution_receipts_pending
    ON workspace_execution_receipts (session_id, state)
    WHERE state IN ('dispatching', 'running', 'unknown');

CREATE TABLE workspace_execution_artifacts (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    session_id UUID NOT NULL,
    preservation_revision BIGINT NOT NULL CHECK (preservation_revision > 0),
    source_path TEXT NOT NULL CHECK (source_path <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    mimetype TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256 BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    CONSTRAINT workspace_execution_artifacts_scope FOREIGN KEY
        (session_id, organization_id, owner_id)
        REFERENCES workspace_execution_sessions (id, organization_id, owner_id),
    CONSTRAINT workspace_execution_artifacts_result_key
        UNIQUE (session_id, preservation_revision, source_path),
    CHECK (octet_length(data) = size_bytes),
    CHECK (expires_at IS NULL OR expires_at > created_at)
);

COMMENT ON TABLE workspace_execution_artifacts IS
    'Complete execution results independent of workspace lifetime.';
COMMENT ON COLUMN workspace_execution_artifacts.expires_at IS
    'Explicit effective retention boundary. NULL makes no time-based expiry promise.';

CREATE TABLE chat_submissions (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    actor_id uuid NOT NULL REFERENCES users(id),
    owner_id uuid NOT NULL REFERENCES users(id),
    request_id uuid NOT NULL,
    input_digest bytea NOT NULL CHECK (octet_length(input_digest) = 32),
    kind text NOT NULL CHECK (kind IN ('create', 'message')),
    -- Creation reserves its identity before the chat and admission hooks exist.
    -- No foreign key: the tombstone must survive deletion of the chat.
    chat_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'accepted', 'uncertain', 'rejected')),
    error text NOT NULL DEFAULT '',
    settings jsonb NOT NULL DEFAULT 'null',
    message_id bigint,
    queued_message_id bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, actor_id, request_id)
);

CREATE INDEX chat_submissions_chat_message_idx
    ON chat_submissions (chat_id, message_id) WHERE message_id IS NOT NULL;
CREATE INDEX chat_submissions_chat_queue_idx
    ON chat_submissions (chat_id, queued_message_id) WHERE queued_message_id IS NOT NULL;

COMMENT ON TABLE chat_submissions IS 'Durable admission identities. Reserved identities are never automatically redispatched, including after server restart.';
