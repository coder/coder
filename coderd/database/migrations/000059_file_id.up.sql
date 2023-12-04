-- This migration updates the files table to move the unique
-- constraint to be hash + created_by. This is necessary to
-- allow regular users who have been granted admin to a specific
-- template to be able to push and read files used for template
-- versions they create.
-- Prior to this collisions on file.hash were not an issue
-- since users who could push files could also read all files.
--
-- This migration also adds a 'files.id' column as the primary
-- key. As a side effect the provisioner_jobs must now reference
-- the files.id column since the 'hash' column is now ambiguous.
-- Drop the primary key on hash.
ALTER TABLE files DROP CONSTRAINT files_pkey;

-- Add an 'id' column and designate it the primary key.
ALTER TABLE files ADD COLUMN
	id uuid NOT NULL PRIMARY KEY DEFAULT gen_random_uuid ();

-- Update the constraint to include the user who created it.
ALTER TABLE files ADD UNIQUE(hash, created_by);

-- Update provisioner_jobs to include a file_id column.
-- This must be temporarily nullable.
ALTER TABLE provisioner_jobs ADD COLUMN file_id uuid;

-- Update all the rows to point to key in the files table.
UPDATE provisioner_jobs
SET
	file_id = files.id
FROM
	files
WHERE
	provisioner_jobs.storage_source = files.hash;

-- Enforce NOT NULL on file_id now.
ALTER TABLE provisioner_jobs ALTER COLUMN file_id SET NOT NULL;
-- Drop storage_source since it is no longer useful for anything.
ALTER TABLE provisioner_jobs DROP COLUMN storage_source;
