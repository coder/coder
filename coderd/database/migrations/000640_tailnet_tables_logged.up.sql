-- Make the tailnet coordination tables LOGGED again.
--
-- UNLOGGED tables are not replicated to standbys and are emptied on
-- promotion or crash recovery. pgcoord writes peer and tunnel rows only
-- when they change and re-registers its ServerTailnet peer only at
-- startup, so after a Postgres failover the rows never came back and
-- replicas kept serving traffic with no coordination state.
--
-- Parent first: a LOGGED child cannot reference an UNLOGGED parent.
ALTER TABLE tailnet_coordinators SET LOGGED;
ALTER TABLE tailnet_peers SET LOGGED;
ALTER TABLE tailnet_tunnels SET LOGGED;
