UPDATE parameter_schemas SET default_source_scheme = 'none' WHERE default_source_scheme IS NULL;
ALTER TABLE parameter_schemas ALTER COLUMN default_source_scheme SET NOT NULL;

UPDATE parameter_schemas SET default_destination_scheme = 'none' WHERE default_destination_scheme IS NULL;
ALTER TABLE parameter_schemas ALTER COLUMN default_destination_scheme SET NOT NULL;
