-- Remove the column from the table first (must happen before dropping the enum type)
ALTER TABLE template_version_presets DROP COLUMN prebuild_status;

-- Then drop the enum type
DROP TYPE prebuild_status;
