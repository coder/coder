ALTER TABLE aibridge_interceptions ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN aibridge_interceptions.annotations IS 'Server-derived annotations captured when the interception was recorded, such as the capabilities the initiator held at that time. Distinct from metadata, which is supplied by the client or provider.';
