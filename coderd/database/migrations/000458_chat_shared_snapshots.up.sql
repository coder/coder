-- Shareable chat snapshots allow chat owners to generate a public,
-- token-protected link that exposes a read-only view of a chat's
-- conversation history and metadata without requiring authentication.
CREATE TABLE chat_shared_snapshots (
	id              UUID        NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	token           TEXT        NOT NULL UNIQUE,
	chat_id         UUID        NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
	owner_id        UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	chat_title      TEXT        NOT NULL DEFAULT '',
	chat_status     chat_status NOT NULL DEFAULT 'waiting',
	messages        JSONB       NOT NULL,
	snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	expires_at      TIMESTAMPTZ,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE chat_shared_snapshots IS 'Self-contained, read-only snapshots of chat state shared via unguessable token links.';
COMMENT ON COLUMN chat_shared_snapshots.token IS 'Crypto-random token used in the public share URL.';
COMMENT ON COLUMN chat_shared_snapshots.messages IS 'Denormalized conversation history at snapshot time as JSON array of ChatMessage objects.';
COMMENT ON COLUMN chat_shared_snapshots.snapshot_at IS 'When the chat state was captured into this snapshot.';
COMMENT ON COLUMN chat_shared_snapshots.expires_at IS 'Optional expiry after which the snapshot link returns 410 Gone.';

CREATE INDEX idx_chat_shared_snapshots_chat_id ON chat_shared_snapshots (chat_id);
CREATE INDEX idx_chat_shared_snapshots_token ON chat_shared_snapshots (token);
