-- Backfill any NULL data values before restoring NOT NULL would require
-- reading from the object store, which is not possible in a migration.
-- Instead, delete rows that only exist in the object store.
DELETE FROM chat_files WHERE data IS NULL;

ALTER TABLE chat_files ALTER COLUMN data SET NOT NULL;
ALTER TABLE chat_files DROP COLUMN object_store_key;
