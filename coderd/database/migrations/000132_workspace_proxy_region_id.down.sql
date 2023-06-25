ALTER TABLE workspace_proxies
	DROP CONSTRAINT workspace_proxies_region_id_unique,
	DROP COLUMN region_id,
	DROP COLUMN derp_enabled;
