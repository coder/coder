ALTER TYPE user_status ADD VALUE IF NOT EXISTS 'dormant';
COMMENT ON TYPE user_status IS 'Defines the user status: active, dormant, or suspended.';
