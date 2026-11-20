-- Children first: an UNLOGGED child may reference a LOGGED parent, but
-- a LOGGED child cannot reference an UNLOGGED parent.
ALTER TABLE tailnet_tunnels SET UNLOGGED;
ALTER TABLE tailnet_peers SET UNLOGGED;
ALTER TABLE tailnet_coordinators SET UNLOGGED;
