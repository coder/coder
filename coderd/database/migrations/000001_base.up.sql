-- This migration creates tables and types for v1 if they do not exist.
-- This allows v2 to operate independently of v1, but share data if it exists.
-- 
-- All tables and types are stolen from:
-- https://github.com/coder/m/blob/47b6fc383347b9f9fab424d829c482defd3e1fe2/product/coder/pkg/database/dump.sql

DO $$ BEGIN
    CREATE TYPE login_type AS ENUM (
        'built-in',
        'saml',
        'oidc'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE userstatus AS ENUM (
        'active',
        'dormant',
        'decommissioned'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS users (
    id text NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    revoked boolean NOT NULL,
    login_type login_type NOT NULL,
    hashed_password bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    temporary_password boolean DEFAULT false NOT NULL,
    avatar_hash text DEFAULT '' :: text NOT NULL,
    ssh_key_regenerated_at timestamp with time zone DEFAULT now() NOT NULL,
    username text DEFAULT '' :: text NOT NULL,
    dotfiles_git_uri text DEFAULT '' :: text NOT NULL,
    roles text [] DEFAULT '{site-member}' :: text [] NOT NULL,
    status userstatus DEFAULT 'active' :: public.userstatus NOT NULL,
    relatime timestamp with time zone DEFAULT now() NOT NULL,
    gpg_key_regenerated_at timestamp with time zone DEFAULT now() NOT NULL,
    _decomissioned boolean DEFAULT false NOT NULL,
    shell text DEFAULT '' :: text NOT NULL
);

CREATE TABLE IF NOT EXISTS organizations (
    id text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    "default" boolean DEFAULT false NOT NULL,
    auto_off_threshold bigint DEFAULT '28800000000000' :: bigint NOT NULL,
    cpu_provisioning_rate real DEFAULT 4.0 NOT NULL,
    memory_provisioning_rate real DEFAULT 1.0 NOT NULL,
    workspace_auto_off boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS organization_members (
    organization_id text NOT NULL,
    user_id text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    roles text [] DEFAULT '{organization-member}' :: text [] NOT NULL
);

CREATE TABLE IF NOT EXISTS api_keys (
    id text NOT NULL,
    hashed_secret bytea NOT NULL,
    user_id text NOT NULL,
    application boolean NOT NULL,
    name text NOT NULL,
    last_used timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    login_type login_type NOT NULL,
    oidc_access_token text DEFAULT ''::text NOT NULL,
    oidc_refresh_token text DEFAULT ''::text NOT NULL,
    oidc_id_token text DEFAULT ''::text NOT NULL,
    oidc_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
    devurl_token boolean DEFAULT false NOT NULL
);

CREATE TABLE IF NOT EXISTS licenses (
    id integer NOT NULL,
    license jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL
);
