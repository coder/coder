ALTER TABLE template_resource_runs ADD COLUMN plan_preview jsonb;
COMMENT ON COLUMN template_resource_runs.plan_preview IS 'Sanitized output names and change metadata only; never raw Terraform plans or output values.';
