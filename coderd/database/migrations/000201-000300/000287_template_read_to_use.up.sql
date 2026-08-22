-- With the "use" verb now existing for templates, we need to update the acl's to
-- include "use" where the permissions set ["read"] is present.
-- The other permission set is ["*"] which is unaffected.

UPDATE
	templates
SET
	-- Instead of trying to write a complicated SQL query to update the JSONB
	-- object, a string replace is much simpler and easier to understand.
	-- Both pieces of text are JSON arrays, so this safe to do.
	group_acl = replace(group_acl::text, '["read"]', '["read", "use"]')::jsonb,
	user_acl = replace(user_acl::text, '["read"]', '["read", "use"]')::jsonb
