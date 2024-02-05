ALTER TABLE workspace_proxies
	ADD COLUMN "derp_only" BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_proxies.derp_only IS 'Disables app/terminal proxying for this proxy and only acts as a DERP relay.';
