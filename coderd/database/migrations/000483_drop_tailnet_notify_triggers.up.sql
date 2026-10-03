DROP TRIGGER IF EXISTS tailnet_notify_peer_change ON tailnet_peers;
DROP TRIGGER IF EXISTS tailnet_notify_tunnel_change ON tailnet_tunnels;
DROP TRIGGER IF EXISTS tailnet_notify_coordinator_heartbeat ON tailnet_coordinators;
DROP FUNCTION IF EXISTS tailnet_notify_peer_change();
DROP FUNCTION IF EXISTS tailnet_notify_tunnel_change();
DROP FUNCTION IF EXISTS tailnet_notify_coordinator_heartbeat();
