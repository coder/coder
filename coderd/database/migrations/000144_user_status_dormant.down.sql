-- It's not possible to drop enum values from enum types, so the UP has "IF NOT EXISTS"

UPDATE users SET status = 'active'::user_status WHERE status::text = 'dormant';
