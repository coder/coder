CREATE TABLE chat_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL DEFAULT '',
    mimetype TEXT NOT NULL,
    data BYTEA NOT NULL
);

CREATE INDEX idx_chat_files_owner ON chat_files(owner_id);
CREATE INDEX idx_chat_files_org ON chat_files(organization_id);
