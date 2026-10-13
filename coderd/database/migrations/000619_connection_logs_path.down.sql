ALTER TABLE connection_logs
	DROP COLUMN IF EXISTS connect_path,
	DROP COLUMN IF EXISTS disconnect_path;

DROP TYPE IF EXISTS connection_path;
