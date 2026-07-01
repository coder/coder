ALTER TABLE boundary_logs DROP CONSTRAINT IF EXISTS boundary_logs_owner_id_fkey;
ALTER TABLE boundary_logs DROP COLUMN IF EXISTS owner_id;
