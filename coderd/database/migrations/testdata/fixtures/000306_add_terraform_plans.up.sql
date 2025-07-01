insert into
	template_version_terraform_values (
		template_version_id,
		cached_plan,
		updated_at
	)
	select
		id,
		'{}',
		now()
	from
		template_versions;
