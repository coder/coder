UPDATE template_version_parameters SET validation_min = 0 WHERE validation_min = NULL;
UPDATE template_version_parameters SET validation_max = 0 WHERE validation_max = NULL;
ALTER TABLE template_version_parameters ALTER COLUMN validation_min SET NOT NULL;
ALTER TABLE template_version_parameters ALTER COLUMN validation_max SET NOT NULL;
