CREATE TYPE aibridge_interception_organization_source AS ENUM (
    'workspace',
    'chatd',
    'effective_group'
);

-- RESTRICT: organizations are soft-deleted, so this only blocks a manual hard
-- delete, and attribution must never silently vanish.
ALTER TABLE aibridge_interceptions
    ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT,
    ADD COLUMN organization_source aibridge_interception_organization_source,
    ADD CONSTRAINT aibridge_interceptions_organization_source_check
        CHECK ((organization_id IS NULL) = (organization_source IS NULL));

COMMENT ON COLUMN aibridge_interceptions.organization_id IS 'The organization the interception was attributed to. NULL when no organization was resolved.';
COMMENT ON COLUMN aibridge_interceptions.organization_source IS 'How organization_id was chosen. NULL exactly when organization_id is NULL.';
