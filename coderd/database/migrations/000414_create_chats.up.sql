CREATE TABLE chats (
	id              UUID        NOT NULL PRIMARY KEY,
	created_at      TIMESTAMPTZ NOT NULL,
	updated_at      TIMESTAMPTZ NOT NULL,
	organization_id UUID        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
	owner_id        UUID        NOT NULL REFERENCES users         (id) ON DELETE CASCADE,
	workspace_id    UUID                 REFERENCES workspaces     (id) ON DELETE RESTRICT,
	title           TEXT,
	provider        TEXT        NOT NULL,
	model           TEXT        NOT NULL,
	metadata        JSONB       NOT NULL DEFAULT '{}'::JSONB
);

CREATE INDEX idx_chats_workspace_id ON chats (workspace_id);

CREATE TABLE chat_messages (
	chat_id    UUID        NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
	id         BIGINT      GENERATED ALWAYS AS IDENTITY,
	created_at TIMESTAMPTZ NOT NULL,
	role       TEXT        NOT NULL,
	content    JSONB       NOT NULL,
	PRIMARY KEY (chat_id, id),
	CONSTRAINT chat_messages_role CHECK (role IN (
		'system',
		'user',
		'assistant',
		'tool_call',
		'tool_result'
	))
);
