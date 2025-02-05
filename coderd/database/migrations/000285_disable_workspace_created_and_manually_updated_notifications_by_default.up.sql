-- Disable 'workspace created' notification by default
UPDATE notification_templates
SET enabled_by_default = FALSE
WHERE id = '281fdf73-c6d6-4cbb-8ff5-888baf8a2fff';

-- Disable 'workspace manually updated' notification by default
UPDATE notification_templates
SET enabled_by_default = FALSE
WHERE id = 'd089fe7b-d5c5-4c0c-aaf5-689859f7d392';
