CREATE FUNCTION tailnet_notify_coordinator_heartbeat() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	PERFORM pg_notify('tailnet_coordinator_heartbeat', NEW.id::text);
	RETURN NULL;
END;
$$;

CREATE FUNCTION tailnet_notify_peer_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_peer_update', OLD.id::text);
		RETURN NULL;
	END IF;
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_peer_update', NEW.id::text);
		RETURN NULL;
	END IF;
END;
$$;

CREATE FUNCTION tailnet_notify_tunnel_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_tunnel_update', NEW.src_id || ',' || NEW.dst_id);
		RETURN NULL;
	ELSIF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_tunnel_update', OLD.src_id || ',' || OLD.dst_id);
		RETURN NULL;
	END IF;
END;
$$;

CREATE TRIGGER tailnet_notify_coordinator_heartbeat AFTER INSERT OR UPDATE ON tailnet_coordinators FOR EACH ROW EXECUTE FUNCTION tailnet_notify_coordinator_heartbeat();

CREATE TRIGGER tailnet_notify_peer_change AFTER INSERT OR DELETE OR UPDATE ON tailnet_peers FOR EACH ROW EXECUTE FUNCTION tailnet_notify_peer_change();

CREATE TRIGGER tailnet_notify_tunnel_change AFTER INSERT OR DELETE OR UPDATE ON tailnet_tunnels FOR EACH ROW EXECUTE FUNCTION tailnet_notify_tunnel_change();
