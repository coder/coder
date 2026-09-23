-- Preserve access for existing organizations, including the bootstrapped default.
ALTER TABLE organizations
    ADD COLUMN restrict_models_to_configured boolean NOT NULL DEFAULT false;

ALTER TABLE organizations
    ALTER COLUMN restrict_models_to_configured SET DEFAULT true;

COMMENT ON COLUMN organizations.restrict_models_to_configured IS
    'Restricts direct AI Gateway model access granted by this organization to enabled configured models.';
