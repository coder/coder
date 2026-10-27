ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_artifact';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_artifact:delete';

CREATE TABLE chat_artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid REFERENCES chat_projects(id) ON DELETE SET NULL,
    title text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_artifacts_title_length CHECK (length(title) BETWEEN 1 AND 200)
);

COMMENT ON TABLE chat_artifacts IS 'Versioned files that chats publish. An artifact outlives its chats. The owner sees it, and the members of project_id see it too.';

CREATE INDEX idx_chat_artifacts_owner_id ON chat_artifacts (owner_id);
CREATE INDEX idx_chat_artifacts_project_id ON chat_artifacts (project_id);

CREATE TABLE chat_artifact_versions (
    artifact_id uuid NOT NULL REFERENCES chat_artifacts(id) ON DELETE CASCADE,
    version integer NOT NULL,
    label text NOT NULL DEFAULT '',
    name text NOT NULL,
    mimetype text NOT NULL,
    data bytea NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    chat_id uuid REFERENCES chats(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (artifact_id, version),
    CONSTRAINT chat_artifact_versions_version_positive CHECK (version > 0),
    CONSTRAINT chat_artifact_versions_label_length CHECK (length(label) <= 60)
);

COMMENT ON COLUMN chat_artifact_versions.name IS 'The file name. Its extension picks the renderer.';
COMMENT ON COLUMN chat_artifact_versions.chat_id IS 'The chat that published this version. NULL after that chat is deleted.';
