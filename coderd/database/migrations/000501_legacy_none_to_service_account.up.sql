-- Convert legacy login_type='none' users into service accounts. Service
-- accounts must have an empty email (see 000433), so this blanks the email of
-- each converted user. Destructive and one-way. System users skipped.
UPDATE users
SET is_service_account = true, email = ''
WHERE login_type = 'none' AND is_service_account = false AND is_system = false;
