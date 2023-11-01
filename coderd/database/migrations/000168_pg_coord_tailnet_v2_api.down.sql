BEGIN;

DROP TRIGGER IF EXISTS tailnet_notify_tunnel_change ON tailnet_tunnels;
DROP FUNCTION IF EXISTS tailnet_notify_tunnel_change;
DROP TABLE IF EXISTS tailnet_tunnels;

DROP TRIGGER IF EXISTS tailnet_notify_peer_change ON tailnet_peers;
DROP FUNCTION IF EXISTS tailnet_notify_peer_change;
DROP INDEX IF EXISTS idx_tailnet_peers_coordinator;
DROP TABLE IF EXISTS tailnet_peers;

DROP TYPE IF EXISTS tailnet_status;

COMMIT;
