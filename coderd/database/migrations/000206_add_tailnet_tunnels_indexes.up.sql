-- Since src_id and dst_id are UUIDs, we only ever compare them with equality, so hash is better
CREATE INDEX idx_tailnet_tunnels_src_id ON tailnet_tunnels USING hash (src_id);
CREATE INDEX idx_tailnet_tunnels_dst_id ON tailnet_tunnels USING hash (dst_id);
