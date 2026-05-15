ALTER TABLE template_version_dlp_policies
DROP COLUMN IF EXISTS desktop_access;

-- enum values can't be dropped, leave 'desktop' on display_app and connection_type.
