ALTER TABLE aibridge_interceptions
    ADD COLUMN client VARCHAR(64)
	DEFAULT 'Unknown';

CREATE INDEX idx_aibridge_interceptions_client ON aibridge_interceptions (client);
