-- temporarily create index column as nullable
ALTER TABLE parameter_schemas ADD COLUMN index integer;

-- initialize ordering for existing parameters
WITH tmp AS (
	SELECT id, row_number() OVER (PARTITION BY job_id ORDER BY name) AS index
	FROM parameter_schemas
)
UPDATE parameter_schemas
SET index = tmp.index
FROM tmp
WHERE parameter_schemas.id = tmp.id;

-- all rows should now be initialized
ALTER TABLE parameter_schemas ALTER COLUMN index SET NOT NULL;
