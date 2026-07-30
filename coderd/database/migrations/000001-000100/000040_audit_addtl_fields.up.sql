ALTER TABLE audit_logs
    ADD COLUMN additional_fields jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN request_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,
    ADD COLUMN resource_icon text NOT NULL DEFAULT '';

ALTER TABLE audit_logs
    ALTER COLUMN additional_fields DROP DEFAULT,
    ALTER COLUMN request_id DROP DEFAULT,
    ALTER COLUMN resource_icon DROP DEFAULT;
