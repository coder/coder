-- GetTemplateAverageBuildTime reads the newest builds of each template
-- version. Without this index it scans every workspace build.
CREATE INDEX workspace_builds_template_version_id_created_at_idx
	ON workspace_builds (template_version_id, created_at DESC);
