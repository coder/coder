-- Preserve chat history when API keys are deleted. Pending work whose latest
-- user turn loses this attribution will fail closed under AI Gateway routing;
-- operators can retry the turn or temporarily use direct routing.
ALTER TABLE chat_messages
ADD COLUMN api_key_id text REFERENCES api_keys(id) ON DELETE SET NULL;

ALTER TABLE chat_queued_messages
ADD COLUMN api_key_id text REFERENCES api_keys(id) ON DELETE SET NULL;
