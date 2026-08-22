ALTER TABLE template_version_parameters ADD COLUMN validation_monotonic text NOT NULL DEFAULT '';

ALTER TABLE template_version_parameters ADD CONSTRAINT validation_monotonic_order CHECK (validation_monotonic IN ('increasing', 'decreasing', ''));

COMMENT ON COLUMN template_version_parameters.validation_monotonic
IS 'Validation: consecutive values preserve the monotonic order';
