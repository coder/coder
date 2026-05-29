ALTER TABLE boundary_sessions DROP CONSTRAINT IF EXISTS boundary_sessions_owner_id_fkey;
ALTER TABLE boundary_sessions DROP COLUMN IF EXISTS owner_id;
