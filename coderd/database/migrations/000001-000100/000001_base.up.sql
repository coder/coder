-- This migration creates tables and types for v1 if they do not exist.
-- This allows v2 to operate independently of v1, but share data if it exists.
-- 
-- All tables and types are stolen from:
-- https://github.com/coder/m/blob/47b6fc383347b9f9fab424d829c482defd3e1fe2/product/coder/pkg/database/dump.sql

CREATE TYPE login_type AS ENUM (
    'password',
    'github'
);

CREATE TABLE IF NOT EXISTS users (
    id uuid NOT NULL,
    email text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    hashed_password bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users USING btree (email);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users USING btree (username);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users USING btree (lower(username));

CREATE TABLE IF NOT EXISTS organizations (
    id uuid NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name ON organizations USING btree (name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name_lower ON organizations USING btree (lower(name));

CREATE TABLE IF NOT EXISTS organization_members (
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    roles text [] DEFAULT '{organization-member}' :: text [] NOT NULL,
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX idx_organization_member_organization_id_uuid ON organization_members USING btree (organization_id);
CREATE INDEX idx_organization_member_user_id_uuid ON organization_members USING btree (user_id);

ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_organization_id_uuid_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY organization_members
    ADD CONSTRAINT organization_members_user_id_uuid_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS api_keys (
    id text NOT NULL,
    hashed_secret bytea NOT NULL,
    user_id uuid NOT NULL,
    last_used timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    login_type login_type NOT NULL,
    oauth_access_token text DEFAULT ''::text NOT NULL,
    oauth_refresh_token text DEFAULT ''::text NOT NULL,
    oauth_id_token text DEFAULT ''::text NOT NULL,
    oauth_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys USING btree (user_id);

ALTER TABLE ONLY api_keys
    ADD CONSTRAINT api_keys_user_id_uuid_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS licenses (
    id serial,
    license jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (id)
);
