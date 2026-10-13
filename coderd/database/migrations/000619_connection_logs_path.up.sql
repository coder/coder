CREATE TYPE connection_path AS ENUM (
	'derp',
	'direct'
);

ALTER TABLE connection_logs
	ADD COLUMN connect_path connection_path DEFAULT NULL,
	ADD COLUMN disconnect_path connection_path DEFAULT NULL;

COMMENT ON COLUMN connection_logs.connect_path IS 'The network path between the client and the agent when the connection was opened. Null for web connections, or when the agent did not know or report the path.';
COMMENT ON COLUMN connection_logs.disconnect_path IS 'The network path between the client and the agent when the connection was closed. Null for web connections, until we receive a disconnect event, or when the agent did not know or report the path.';
