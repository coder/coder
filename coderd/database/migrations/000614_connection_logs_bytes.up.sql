ALTER TABLE connection_logs
	ADD COLUMN rx_bytes bigint DEFAULT NULL,
	ADD COLUMN tx_bytes bigint DEFAULT NULL;

COMMENT ON COLUMN connection_logs.rx_bytes IS 'Total bytes received by the agent on this connection. Null for web events and non-disconnect events.';
COMMENT ON COLUMN connection_logs.tx_bytes IS 'Total bytes sent by the agent on this connection. Null for web events and non-disconnect events.';
