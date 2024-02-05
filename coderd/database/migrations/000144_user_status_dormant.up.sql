CREATE TYPE new_user_status AS ENUM (
	'active',
	'suspended',
	'dormant'
);
COMMENT ON TYPE new_user_status IS 'Defines the users status: active, dormant, or suspended.';

ALTER TABLE users
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE new_user_status USING (status::text::new_user_status),
	ALTER COLUMN status SET DEFAULT 'active'::new_user_status;

DROP TYPE user_status;
ALTER TYPE new_user_status RENAME TO user_status;
