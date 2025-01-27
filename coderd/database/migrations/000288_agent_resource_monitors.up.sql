CREATE TYPE workspace_agent_monitored_resource_type AS ENUM ('memory', 'volume');


CREATE TABLE workspace_agent_resource_monitors (
	agent_id uuid NOT NULL,
	rtype workspace_agent_monitored_resource_type NOT NULL,
	enabled boolean NOT NULL,
	threshold integer NOT NULL,
	metadata jsonb DEFAULT'{}'::jsonb NOT NULL,
	created_at timestamp with time zone NOT NULL
);
