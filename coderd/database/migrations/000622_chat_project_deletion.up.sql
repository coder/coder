ALTER TABLE chat_projects ADD COLUMN deleted boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN chat_projects.deleted IS 'Irreversible. Its memories and instructions are deleted with the tombstone. The project''s archived chats follow chat retention (kept forever when retention is 0); dbpurge deletes the row once none are left.';

-- Project queries match sub-chats on project_id alone. The trigger sets it on
-- every sub-chat insert, including from older replicas during a rolling
-- upgrade.
CREATE FUNCTION set_sub_chat_project_id() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.root_chat_id IS NOT NULL THEN
        SELECT project_id INTO NEW.project_id
        FROM chats
        WHERE id = NEW.root_chat_id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_set_sub_chat_project_id
    BEFORE INSERT ON chats
    FOR EACH ROW
    EXECUTE FUNCTION set_sub_chat_project_id();

UPDATE chats
SET project_id = root.project_id
FROM chats root
WHERE root.id = chats.root_chat_id
    AND root.project_id IS NOT NULL
    AND chats.project_id IS NULL;

COMMENT ON COLUMN chats.project_id IS 'Sub-chats carry their root chat''s project; an insert trigger sets it and replaces any passed value.';

-- Lets the deleted-project sweep skip projects whose chats are all archived.
CREATE INDEX idx_chats_project_id_unarchived ON chats (project_id) WHERE NOT archived;
