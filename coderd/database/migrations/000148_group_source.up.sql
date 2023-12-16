CREATE TYPE group_source AS ENUM (
    -- User created groups
	'user',
    -- Groups created by the system through oidc sync
	'oidc'
);

ALTER TABLE groups
	ADD COLUMN source group_source NOT NULL DEFAULT 'user';

COMMENT ON COLUMN groups.source IS 'Source indicates how the group was created. It can be created by a user manually, or through some system process like OIDC group sync.';
