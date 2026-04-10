-- Add object_store_key to track files stored in external object storage.
-- When non-NULL, the file data lives in the object store under this key
-- and the data column may be NULL.
ALTER TABLE chat_files ADD COLUMN object_store_key TEXT;

-- Make data nullable so new writes can skip the BYTEA column when
-- storing in the object store.
ALTER TABLE chat_files ALTER COLUMN data DROP NOT NULL;
