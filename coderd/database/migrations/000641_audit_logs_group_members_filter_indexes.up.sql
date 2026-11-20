-- Audit log filters on action or resource_type otherwise walk
-- idx_audit_logs_time_desc (or scan the table) until a page of matches is
-- found, which reads the whole table for rare values. Single column indexes
-- stay small through btree deduplication; frequent values still use the time
-- index. They also serve the capped count and the dbpurge connection event
-- cleanup, which filters on action.
CREATE INDEX idx_audit_logs_action ON audit_logs (action);
CREATE INDEX idx_audit_logs_resource_type ON audit_logs (resource_type);

-- group_members is only indexed by (user_id, group_id), so lookups by group
-- through group_members_expanded and the cascade from groups scanned the whole
-- table.
CREATE INDEX idx_group_members_group_id ON group_members (group_id);
