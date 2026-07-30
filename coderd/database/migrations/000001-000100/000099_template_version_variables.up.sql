CREATE TABLE IF NOT EXISTS template_version_variables (
    template_version_id uuid not null references template_versions (id) on delete cascade,
    name text not null,
    description text not null,
    type text not null,
    value text not null,
    default_value text not null,
    required boolean not null,
    sensitive boolean not null,
    unique (template_version_id, name)
);

COMMENT ON COLUMN template_version_variables.name IS 'Variable name';
COMMENT ON COLUMN template_version_variables.description IS 'Variable description';
COMMENT ON COLUMN template_version_variables.type IS 'Variable type';
COMMENT ON COLUMN template_version_variables.value IS 'Variable value';
COMMENT ON COLUMN template_version_variables.default_value IS 'Variable default value';
COMMENT ON COLUMN template_version_variables.required IS 'Required variables needs a default value or a value provided by template admin';
COMMENT ON COLUMN template_version_variables.sensitive IS 'Sensitive variables have their values redacted in logs or site UI';
