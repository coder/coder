CREATE TYPE tailnet_status AS ENUM (
	'ok',
	'lost'
);

CREATE TABLE tailnet_peers (
	id uuid NOT NULL,
	coordinator_id uuid NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	node bytea NOT NULL,
	status tailnet_status DEFAULT 'ok'::tailnet_status NOT NULL,
	PRIMARY KEY (id, coordinator_id),
	FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators(id) ON DELETE CASCADE
);

-- For shutting down / GC a coordinator
CREATE INDEX idx_tailnet_peers_coordinator ON tailnet_peers (coordinator_id);

-- Any time tailnet_peers table changes, send an update with the affected peer ID.
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

CREATE TRIGGER tailnet_notify_peer_change
	AFTER INSERT OR UPDATE OR DELETE ON tailnet_peers
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_peer_change();

CREATE TABLE tailnet_tunnels (
	coordinator_id uuid NOT NULL,
	-- we don't keep foreign keys for src_id and dst_id because the coordinator doesn't
	-- strictly order creating the peers and creating the tunnels
	src_id uuid NOT NULL,
	dst_id uuid NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	PRIMARY KEY (coordinator_id, src_id, dst_id),
	FOREIGN KEY (coordinator_id)  REFERENCES tailnet_coordinators (id) ON DELETE CASCADE
);

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

CREATE TRIGGER tailnet_notify_tunnel_change
	AFTER INSERT OR UPDATE OR DELETE ON tailnet_tunnels
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_tunnel_change();
