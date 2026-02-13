-- Remove unused tailnet v1 API tables.
-- These tables were superseded by tailnet_peers and tailnet_tunnels in migration
-- 000168. The v1 API code was removed in commit d6154c4310 ("remove tailnet v1
-- API support"), but the tables and queries were never cleaned up.

-- Drop triggers first (they reference the functions).
DROP TRIGGER IF EXISTS tailnet_notify_agent_change ON tailnet_agents;
DROP TRIGGER IF EXISTS tailnet_notify_client_change ON tailnet_clients;
DROP TRIGGER IF EXISTS tailnet_notify_client_subscription_change ON tailnet_client_subscriptions;

-- Drop the trigger functions.
DROP FUNCTION IF EXISTS tailnet_notify_agent_change();
DROP FUNCTION IF EXISTS tailnet_notify_client_change();
DROP FUNCTION IF EXISTS tailnet_notify_client_subscription_change();

-- Drop the tables. Foreign keys and indexes are dropped automatically via CASCADE.
-- Order matters due to potential foreign key relationships.
DROP TABLE IF EXISTS tailnet_client_subscriptions;
DROP TABLE IF EXISTS tailnet_agents;
DROP TABLE IF EXISTS tailnet_clients;
