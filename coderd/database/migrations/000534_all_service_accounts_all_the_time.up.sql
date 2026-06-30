-- Convert legacy users created with login_type 'none' into service accounts.
-- Service accounts require empty email per users_email_not_empty.
UPDATE users
SET is_service_account = true,
	email = ''
WHERE login_type = 'none'
	AND is_service_account = false
	-- `prebuilds@system` user should not convert it to service account.
	AND is_system = false;
