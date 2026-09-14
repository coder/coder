-- drop any rows that aren't primary replicas
DELETE FROM replicas
	WHERE "primary" = false;

ALTER TABLE replicas
	DROP COLUMN "primary";

ALTER TABLE workspace_proxies
	DROP CONSTRAINT workspace_proxies_region_id_unique,
	DROP COLUMN region_id,
	DROP COLUMN derp_enabled;
