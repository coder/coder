BEGIN;
	ALTER TABLE template_versions ALTER COLUMN created_by SET NOT NULL;
	UPDATE template_versions SET created_by = '00000000-0000-0000-0000-000000000000'::uuid WHERE created_by IS NULL;
COMMIT;
