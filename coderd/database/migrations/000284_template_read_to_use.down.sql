UPDATE
	templates
SET
	group_acl = replace(group_acl::text, '["read", "use"]', '["read"]')::jsonb,
	user_acl = replace(user_acl::text, '["read", "use"]', '["read"]')::jsonb
