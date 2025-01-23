CREATE TYPE resource_monitoring_type AS ENUM ('memory', 'volume');


CREATE TABLE agent_resources_monitoring (
	agent_id uuid NOT NULL,
	rtype resource_monitoring_type NOT NULL,
	enabled boolean NOT NULL,
	threshold integer NOT NULL,
	created_at timestamp with time zone NOT NULL,
	updated_at timestamp with time zone NOT NULL
);
