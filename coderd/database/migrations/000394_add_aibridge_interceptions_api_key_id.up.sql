  -- column is nullable to not break interceptions recorded before this column was added
ALTER TABLE aibridge_interceptions ADD COLUMN api_key_id text;
