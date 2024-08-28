ALTER TABLE notification_messages
ADD COLUMN org_id UUID NULL REFERENCES organizations (id);