CREATE TABLE IF NOT EXISTS template_version_parameters (
    template_version_id uuid not null references template_versions (id) on delete cascade,
    name text not null,
    description text not null,
    type text not null,
    mutable boolean not null,
    default_value text not null,
    icon text not null,
    options jsonb not null default '[]'::jsonb,
    validation_regex text not null,
    validation_min integer not null,
    validation_max integer not null,
    unique (template_version_id, name)
);

COMMENT ON COLUMN template_version_parameters.name IS 'Parameter name';
COMMENT ON COLUMN template_version_parameters.description IS 'Parameter description';
COMMENT ON COLUMN template_version_parameters.type IS 'Parameter type';
COMMENT ON COLUMN template_version_parameters.mutable IS 'Is parameter mutable?';
COMMENT ON COLUMN template_version_parameters.default_value IS 'Default value';
COMMENT ON COLUMN template_version_parameters.icon IS 'Icon';
COMMENT ON COLUMN template_version_parameters.options IS 'Additional options';
COMMENT ON COLUMN template_version_parameters.validation_regex IS 'Validation: regex pattern';
COMMENT ON COLUMN template_version_parameters.validation_min IS 'Validation: minimum length of value';
COMMENT ON COLUMN template_version_parameters.validation_max IS 'Validation: maximum length of value';

CREATE TABLE IF NOT EXISTS workspace_build_parameters (
    workspace_build_id uuid not null references workspace_builds (id) on delete cascade,
    name text not null,
    value text not null,
    unique (workspace_build_id, name)
);

COMMENT ON COLUMN workspace_build_parameters.name IS 'Parameter name';
COMMENT ON COLUMN workspace_build_parameters.value IS 'Parameter value';
