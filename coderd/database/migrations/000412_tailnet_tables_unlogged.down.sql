-- Revert tailnet tables to LOGGED (standard WAL-enabled tables).
-- WARNING: This requires a full table rewrite with WAL generation,
-- which can be slow for large tables.

-- Convert parent table first (before children, reverse of up migration).
ALTER TABLE tailnet_coordinators SET LOGGED;

-- Convert child tables after parent.
ALTER TABLE tailnet_peers SET LOGGED;
ALTER TABLE tailnet_tunnels SET LOGGED;
