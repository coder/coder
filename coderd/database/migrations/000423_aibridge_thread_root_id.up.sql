ALTER TABLE aibridge_interceptions
ADD COLUMN thread_root_id UUID NULL;

COMMENT ON COLUMN aibridge_interceptions.thread_root_id IS 'The root interception of the thread that this interception belongs to.';

CREATE INDEX idx_aibridge_interceptions_thread_root_id ON aibridge_interceptions (thread_root_id);
