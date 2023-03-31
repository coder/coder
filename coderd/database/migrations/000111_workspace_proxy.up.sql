BEGIN;
CREATE TABLE workspace_proxies (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    name text NOT NULL,
    display_name text NOT NULL,
    icon text NOT NULL,
    url text NOT NULL,
    wildcard_url text NOT NULL,
	created_at timestamp with time zone NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	deleted boolean NOT NULL,

    PRIMARY KEY (id)
);

COMMENT ON COLUMN workspace_proxies.url IS 'Full url including scheme of the proxy api url: https://us.example.com';
COMMENT ON COLUMN workspace_proxies.wildcard_url IS 'URL with the wildcard for subdomain based app hosting: https://*.us.example.com';


-- Enforces no active proxies have the same name.
CREATE UNIQUE INDEX ON workspace_proxies (organization_id, name) WHERE deleted = FALSE;

COMMIT;
