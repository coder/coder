-- Convert all tailnet coordination tables to UNLOGGED for improved write performance.
-- These tables contain ephemeral coordination data that can be safely reconstructed
-- after a crash. UNLOGGED tables skip WAL writes, significantly improving performance
-- for high-frequency updates like coordinator heartbeats and peer state changes.
--
-- IMPORTANT: UNLOGGED tables are truncated on crash recovery and are not replicated
-- to standby servers. This is acceptable because:
-- 1. Coordinators re-register on startup
-- 2. Peers re-establish connections on reconnect
-- 3. Tunnels are re-created based on current peer state

-- Convert child tables first (they have FK references to tailnet_coordinators).
-- UNLOGGED child tables can reference LOGGED parent tables, but LOGGED child
-- tables cannot reference UNLOGGED parent tables. So we must convert children
-- before converting the parent.
ALTER TABLE tailnet_tunnels SET UNLOGGED;
ALTER TABLE tailnet_peers SET UNLOGGED;

-- Convert parent table last (after all children are unlogged).
ALTER TABLE tailnet_coordinators SET UNLOGGED;
