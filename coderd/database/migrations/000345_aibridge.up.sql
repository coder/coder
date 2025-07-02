CREATE TABLE wormhole
(
	id         UUID,
	created_at timestamptz NOT NULL,
	event      jsonb       NOT NULL,
	event_type varchar(32) NOT NULL
);
