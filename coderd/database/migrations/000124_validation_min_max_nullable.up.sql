ALTER TABLE template_version_parameters ALTER COLUMN validation_min DROP NOT NULL;
ALTER TABLE template_version_parameters ALTER COLUMN validation_max DROP NOT NULL;
UPDATE template_version_parameters SET validation_min = NULL WHERE validation_min = 0;
UPDATE template_version_parameters SET validation_max = NULL WHERE validation_max = 0;
