INSERT INTO workspace_agent_port_share
	(workspace_id, agent_name, port, share_level)
VALUES
	('b90547be-8870-4d68-8184-e8b2242b7c01', 'qua', 8080, 'public'::app_sharing_level) RETURNING *;
