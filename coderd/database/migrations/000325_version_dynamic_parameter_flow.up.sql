-- Default to `true`. Users will have to opt out of the new flow if they have issues.
ALTER TABLE templates ADD COLUMN dynamic_parameter_flow BOOL NOT NULL DEFAULT true;

COMMENT ON COLUMN templates.dynamic_parameter_flow IS
	'Determines whether to default to the dynamic parameter creation flow for this template. '
	'As a template wide setting, the template admin can opt out if there are any issues. '
	'An escape hatch is required, as workspace creation is a core workflow and cannot break. '
	'This column will be removed when the dynamic parameter creation flow is stable.';


