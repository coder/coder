-- 000563 copied this value into templates.agents_allowed. Deleting the row
-- makes the original list unrecoverable.
DELETE FROM site_configs
WHERE key = 'agents_template_allowlist';
