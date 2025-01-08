-- With the "use" verb now existing for templates, we need to update the acl's to
-- include "use" where the permissions set ["read"] is present.
-- The other permission set is ["*"] which is unaffected.

UPDATE
	templates
SET
	group_acl = replace(group_acl::text, '["read", "use"]', '["read"]')::jsonb,
	user_acl = replace(user_acl::text, '["read", "use"]', '["read"]')::jsonb
