DROP TRIGGER IF EXISTS tailnet_notify_client_change ON tailnet_clients;
DROP FUNCTION IF EXISTS tailnet_notify_client_change;
DROP INDEX IF EXISTS idx_tailnet_clients_agent;
DROP INDEX IF EXISTS idx_tailnet_clients_coordinator;
DROP TABLE tailnet_clients;

DROP TRIGGER IF EXISTS tailnet_notify_agent_change ON tailnet_agents;
DROP FUNCTION IF EXISTS tailnet_notify_agent_change;
DROP INDEX IF EXISTS idx_tailnet_agents_coordinator;
DROP TABLE IF EXISTS tailnet_agents;

DROP TRIGGER IF EXISTS tailnet_notify_coordinator_heartbeat ON tailnet_coordinators;
DROP FUNCTION IF EXISTS tailnet_notify_coordinator_heartbeat;
DROP TABLE IF EXISTS tailnet_coordinators;
