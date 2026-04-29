DROP INDEX IF EXISTS idx_aibridge_interceptions_boundary_session_id;

ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS boundary_sequence_number,
    DROP COLUMN IF EXISTS boundary_session_id;
