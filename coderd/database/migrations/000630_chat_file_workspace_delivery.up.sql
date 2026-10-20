CREATE TYPE chat_file_purpose AS ENUM (
    'prompt',
    'workspace'
);

ALTER TABLE chat_files
    ADD COLUMN purpose chat_file_purpose NOT NULL DEFAULT 'prompt',
    ADD COLUMN workspace_delivery_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN workspace_delivery_next_attempt_at timestamp with time zone,
    ADD COLUMN workspace_delivery_error text,
    ADD COLUMN workspace_delivered_at timestamp with time zone,
    ADD COLUMN workspace_delivered_path text,
    ADD COLUMN workspace_delivered_workspace_id uuid;

COMMENT ON COLUMN chat_files.purpose IS 'Where the file goes: prompt files are sent to the model provider, workspace files are copied into the chat''s workspace and never sent to the provider.';
COMMENT ON COLUMN chat_files.workspace_delivery_attempts IS 'Counted delivery attempts for a workspace file. Delivery stops at the configured maximum until a retry resets it.';
COMMENT ON COLUMN chat_files.workspace_delivery_next_attempt_at IS 'When a workspace file may be claimed next. NULL means due now. A claim sets it to the lease expiry, so a future value without an error means a delivery is in flight.';
COMMENT ON COLUMN chat_files.workspace_delivery_error IS 'Error from the last failed delivery attempt. Cleared when the file is claimed again.';
COMMENT ON COLUMN chat_files.workspace_delivered_at IS 'When the file was written into the workspace. Once set, data is empty and only metadata and the receipt remain.';
COMMENT ON COLUMN chat_files.workspace_delivered_path IS 'Home-relative path the workspace agent wrote the file to.';
COMMENT ON COLUMN chat_files.workspace_delivered_workspace_id IS 'Workspace the file was delivered to.';

CREATE INDEX idx_chat_files_workspace_delivery_due ON chat_files USING btree (workspace_delivery_next_attempt_at)
    WHERE purpose = 'workspace' AND workspace_delivered_at IS NULL;
