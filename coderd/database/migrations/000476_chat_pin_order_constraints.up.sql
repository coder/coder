-- Defensive: fix any existing violating rows before adding constraints.
UPDATE chats SET pin_order = 0
    WHERE pin_order > 0 AND parent_chat_id IS NOT NULL;

UPDATE chats SET pin_order = 0
    WHERE pin_order > 0 AND archived = true;

ALTER TABLE chats
    ADD CONSTRAINT chats_pin_order_parent_check
    CHECK (pin_order = 0 OR parent_chat_id IS NULL);

ALTER TABLE chats
    ADD CONSTRAINT chats_pin_order_archived_check
    CHECK (pin_order = 0 OR archived = false);
