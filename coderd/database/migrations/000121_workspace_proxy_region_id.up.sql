ALTER TABLE workspace_proxies
	-- adding a serial to a table without a default value will be filled as you
	-- would expect
	ADD COLUMN region_id serial NOT NULL,
	ADD COLUMN derp_enabled boolean NOT NULL DEFAULT true,
	ADD CONSTRAINT workspace_proxies_region_id_unique UNIQUE (region_id);
