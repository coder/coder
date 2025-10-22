-- Add back the storage_source column. This must be nullable temporarily.
ALTER TABLE provisioner_jobs ADD COLUMN storage_source text;

-- Set the storage_source to the hash of the files.id reference.
UPDATE
	provisioner_jobs
SET
	storage_source=files.hash
FROM
	files
WHERE
	provisioner_jobs.file_id = files.id;

-- Now that we've populated storage_source drop the file_id column.
ALTER TABLE provisioner_jobs DROP COLUMN file_id;
-- We can set the storage_source column as NOT NULL now.
ALTER TABLE provisioner_jobs ALTER COLUMN storage_source SET NOT NULL;

-- Delete all the duplicate rows where hashes collide.
-- We filter on 'id' to ensure only 1 unique row.
DELETE FROM
	files a
USING
	files b
WHERE
	a.created_by < b.created_by
AND
	a.hash = b.hash;

-- Drop the primary key on files.id.
ALTER TABLE files DROP CONSTRAINT files_pkey;
-- Drop the id column.
ALTER TABLE files DROP COLUMN id;
-- Drop the unique constraint on hash + owner.
ALTER TABLE files DROP CONSTRAINT files_hash_created_by_key;
-- Set the primary key back to hash.
ALTER TABLE files ADD PRIMARY KEY (hash);
