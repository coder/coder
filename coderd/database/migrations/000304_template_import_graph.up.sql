create table template_version_terraform_values (
	template_version_id uuid not null unique references template_versions(id),
	updated_at timestamptz not null default now(),
	cached_plan jsonb not null
);
