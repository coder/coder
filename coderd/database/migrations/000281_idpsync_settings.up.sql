-- Allow modifications to notification templates to be audited.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'idp_sync_settings_organization';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'idp_sync_settings_group';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'idp_sync_settings_role';
