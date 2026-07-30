-- We can't guarantee that an IP will always be available, and omitting an IP
-- is preferable to not creating a connection log at all.
ALTER TABLE connection_logs ALTER COLUMN ip DROP NOT NULL;
