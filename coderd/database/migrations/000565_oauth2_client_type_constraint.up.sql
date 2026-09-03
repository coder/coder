-- client_type decides whether the token endpoint validates a client secret at
-- all. Constrain it at the schema level: no Go path can write a bad value
-- today, but a future migration writing 'public' onto a row that holds a secret
-- would turn off client authentication for that app with nothing to catch it.
--
-- A non-NULL value outside the two canonical strings fails ADD CONSTRAINT and,
-- because all migrations share one transaction, blocks the upgrade. That is
-- deliberate: coercing an unexplained value would hide whatever wrote it.
--
-- This should touch zero rows: migration 000344 added the column with a default
-- of 'confidential' and backfilled existing rows with COALESCE.
UPDATE oauth2_provider_apps SET client_type = 'confidential' WHERE client_type IS NULL;

ALTER TABLE oauth2_provider_apps
    ADD CONSTRAINT oauth2_provider_apps_client_type_check
    CHECK (client_type IN ('confidential', 'public'));

ALTER TABLE oauth2_provider_apps
    ALTER COLUMN client_type SET NOT NULL;
