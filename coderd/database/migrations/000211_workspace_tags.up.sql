CREATE TABLE IF NOT EXISTS template_version_workspace_tags (
    template_version_id uuid not null references template_versions (id) on delete cascade,
    key text not null,
    value text not null,
    unique (template_version_id, key)
);
