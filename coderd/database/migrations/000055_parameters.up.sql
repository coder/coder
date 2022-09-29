CREATE TABLE template_version_parameters (
    template_version_id uuid not null references template_versions (id) on delete cascade,
    name text not null,
    description text not null,
    type text not null,
    immutable boolean not null,
    default_value text not null,
    icon text not null,
    options jsonb not null default '[]'::jsonb,
    validation_regex text,
    validation_min integer,
    validation_max integer,
    unique (template_version_id, name)
);

CREATE TABLE workspace_parameters (
    workspace_id uuid not null,
    name text not null,
    value text not null,
    unique (workspace_id, name)
);
