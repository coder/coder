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


DO $$ BEGIN
    CREATE TYPE rtcmode AS ENUM (
        'auto',
        'turn',
        'stun'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE FUNCTION uuid_to_longid_trigger() RETURNS trigger AS $$
BEGIN
    if NEW.id_old IS NULL THEN
        NEW.id_old := NEW.id::text;
    END if;

    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION user_uuid_to_longid_trigger() RETURNS trigger AS $$
BEGIN
    if NEW.user_id_old IS NULL THEN
        NEW.user_id_old := NEW.user_id::text;
    END if;

    return NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION org_uuid_to_longid_trigger() RETURNS trigger AS $$
BEGIN
    if NEW.organization_id_old IS NULL THEN
        NEW.organization_id_old := NEW.organization_id::text;
    END if;

    return NEW;
END;
$$ LANGUAGE plpgsql;

--
-- Name: users; Type: TABLE; Schema: public; Owner: coder
--

CREATE TABLE IF NOT EXISTS users (
    id uuid NOT NULL,
    id_old text NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    revoked boolean NOT NULL,
    login_type login_type NOT NULL,
    hashed_password bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    temporary_password boolean DEFAULT false NOT NULL,
    avatar_hash text DEFAULT ''::text NOT NULL,
    ssh_key_regenerated_at timestamp with time zone DEFAULT now() NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    dotfiles_git_uri text DEFAULT ''::text NOT NULL,
    roles text[] DEFAULT '{site-member}'::text[] NOT NULL,
    status userstatus DEFAULT 'active'::userstatus NOT NULL,
    relatime timestamp with time zone DEFAULT now() NOT NULL,
    gpg_key_regenerated_at timestamp with time zone DEFAULT now() NOT NULL,
    _decomissioned boolean DEFAULT false NOT NULL,
    shell text DEFAULT ''::text NOT NULL,
    autostart_at timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL,
    rtc_mode rtcmode DEFAULT 'auto'::rtcmode NOT NULL,
    username_pre_dedup text DEFAULT ''::text NOT NULL,
    PRIMARY KEY (id_old)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users USING btree (email);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_id_uuid ON users USING btree (id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users USING btree (username);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users USING btree (lower(username));

CREATE TRIGGER
    trig_uuid_to_longid_users
BEFORE INSERT ON
    users
FOR EACH ROW EXECUTE PROCEDURE
    uuid_to_longid_trigger();

--
-- Name: organizations; Type: TABLE; Schema:  Owner: coder
--

CREATE TABLE IF NOT EXISTS organizations (
    id uuid NOT NULL,
    id_old text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    "default" boolean DEFAULT false NOT NULL,
    auto_off_threshold bigint DEFAULT '28800000000000' :: bigint NOT NULL,
    cpu_provisioning_rate real DEFAULT 4.0 NOT NULL,
    memory_provisioning_rate real DEFAULT 1.0 NOT NULL,
    workspace_auto_off boolean DEFAULT false NOT NULL,
    PRIMARY KEY (id_old)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name ON organizations USING btree (name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_id_uuid ON organizations USING btree (id);

CREATE TRIGGER
    trig_uuid_to_longid_organizations
BEFORE INSERT ON
    organizations
FOR EACH ROW EXECUTE PROCEDURE
    uuid_to_longid_trigger();

-- ALTER TABLE ONLY organizations
--     ADD CONSTRAINT organizations_pkey PRIMARY KEY (id_old);

CREATE TABLE IF NOT EXISTS organization_members (
    organization_id_old text NOT NULL,
    user_id_old text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    roles text [] DEFAULT '{organization-member}' :: text [] NOT NULL,
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    PRIMARY KEY (organization_id, user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_members_user_org_id_uuid ON organization_members USING btree (user_id, organization_id);
CREATE INDEX IF NOT EXISTS idx_organization_member_organization_id ON organization_members USING btree (organization_id_old);
CREATE INDEX idx_organization_member_organization_id_uuid ON organization_members USING btree (organization_id);
CREATE INDEX IF NOT EXISTS idx_organization_member_user_id ON organization_members USING btree (user_id_old);
CREATE INDEX idx_organization_member_user_id_uuid ON organization_members USING btree (user_id);

DO $$ BEGIN
    ALTER TABLE ONLY organization_members
        ADD CONSTRAINT organization_members_organization_id_fkey FOREIGN KEY (organization_id_old) REFERENCES organizations(id_old) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    ALTER TABLE ONLY organization_members
        ADD CONSTRAINT organization_members_user_id_fkey FOREIGN KEY (user_id_old) REFERENCES users(id_old) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    ALTER TABLE ONLY organization_members
        ADD CONSTRAINT organization_members_organization_id_uuid_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    ALTER TABLE ONLY organization_members
        ADD CONSTRAINT organization_members_user_id_uuid_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TRIGGER
    trig_uuid_to_longid_organization_members_user_id
BEFORE INSERT ON
    organization_members
FOR EACH ROW EXECUTE PROCEDURE
    user_uuid_to_longid_trigger();

CREATE TRIGGER
    trig_uuid_to_longid_organization_members_org_id
BEFORE INSERT ON
    organization_members
FOR EACH ROW EXECUTE PROCEDURE
    org_uuid_to_longid_trigger();

CREATE TABLE IF NOT EXISTS api_keys (
    id text NOT NULL,
    hashed_secret bytea NOT NULL,
    user_id_old text NOT NULL,
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
    devurl_token boolean DEFAULT false NOT NULL,
    user_id uuid NOT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys USING btree (user_id_old);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_uuid ON api_keys USING btree (user_id);

DO $$ BEGIN
    ALTER TABLE ONLY api_keys
        ADD CONSTRAINT api_keys_user_id_uuid_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TRIGGER
    trig_uuid_to_longid_api_keys_user_id
BEFORE INSERT ON
    api_keys
FOR EACH ROW EXECUTE PROCEDURE
    user_uuid_to_longid_trigger();

CREATE TABLE IF NOT EXISTS licenses (
    id serial,
    license jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (id)
);
