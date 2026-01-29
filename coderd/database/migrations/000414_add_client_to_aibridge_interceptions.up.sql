ALTER TABLE aibridge_interceptions
    ADD COLUMN client VARCHAR(64);

CREATE INDEX idx_aibridge_interceptions_client ON aibridge_interceptions (client);
