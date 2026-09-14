-- Convert legacy users created with login_type 'none' to password auth.
-- OSS deployments cannot create service accounts without Premium. Existing
-- API tokens remain valid; admins can set a password if password login is
-- desired.
UPDATE users
SET login_type = 'password'
WHERE login_type = 'none'
	AND is_service_account = false
	AND is_system = false;
