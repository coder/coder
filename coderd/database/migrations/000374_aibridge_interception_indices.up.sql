-- This is used for consistent cursor pagination.
CREATE INDEX IF NOT EXISTS idx_aibridge_interceptions_started_id_desc
    ON aibridge_interceptions (started_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_aibridge_interceptions_provider
  ON aibridge_interceptions (provider);

CREATE INDEX IF NOT EXISTS idx_aibridge_interceptions_model
  ON aibridge_interceptions (model);
