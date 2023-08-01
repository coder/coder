-- It's not possible to drop enum values from enum types, so the UP has "IF NOT EXISTS"

UPDATE users SET user_status = 'active'::user_status WHERE user_status = 'dormant'::user_status;
