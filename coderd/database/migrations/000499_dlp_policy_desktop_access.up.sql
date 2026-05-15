ALTER TYPE display_app ADD VALUE IF NOT EXISTS 'desktop';
ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'desktop';

ALTER TABLE template_version_dlp_policies
ADD COLUMN desktop_access BOOLEAN NOT NULL DEFAULT false;
