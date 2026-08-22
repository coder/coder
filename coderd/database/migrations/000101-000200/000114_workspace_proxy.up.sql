CREATE TABLE workspace_proxies (
    id uuid NOT NULL,
    name text NOT NULL,
    display_name text NOT NULL,
    icon text NOT NULL,
    url text NOT NULL,
	wildcard_hostname text NOT NULL,
	created_at timestamp with time zone NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	deleted boolean NOT NULL,

    PRIMARY KEY (id)
);

COMMENT ON COLUMN workspace_proxies.url IS 'Full url including scheme of the proxy api url: https://us.example.com';
COMMENT ON COLUMN workspace_proxies.wildcard_hostname IS 'Hostname with the wildcard for subdomain based app hosting: *.us.example.com';


-- Enforces no active proxies have the same name.
CREATE UNIQUE INDEX ON workspace_proxies (name) WHERE deleted = FALSE;
