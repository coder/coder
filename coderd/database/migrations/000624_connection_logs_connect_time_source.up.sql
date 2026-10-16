CREATE TYPE connection_log_connect_time_source AS ENUM ('unknown', 'connect', 'disconnect');

ALTER TABLE connection_logs
	ADD COLUMN connect_time_source connection_log_connect_time_source NOT NULL DEFAULT 'unknown';

COMMENT ON COLUMN connection_logs.connect_time_source IS 'Source of connect_time: unknown when provenance is unavailable, connect for an observed connect event, disconnect for a standalone disconnect placeholder.';
