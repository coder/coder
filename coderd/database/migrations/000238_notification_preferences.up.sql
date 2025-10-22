CREATE TABLE notification_preferences
(
	user_id                  uuid REFERENCES users ON DELETE CASCADE                  NOT NULL,
	notification_template_id uuid REFERENCES notification_templates ON DELETE CASCADE NOT NULL,
	disabled                 bool                                                     NOT NULL DEFAULT FALSE,
	created_at               TIMESTAMP WITH TIME ZONE                                 NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at               TIMESTAMP WITH TIME ZONE                                 NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, notification_template_id)
);

-- Add a new type (to be expanded upon later) which specifies the kind of notification template.
CREATE TYPE notification_template_kind AS ENUM (
	'system'
	);

ALTER TABLE notification_templates
	-- Allow per-template notification method (enterprise only).
	ADD COLUMN method notification_method,
	-- Update all existing notification templates to be system templates.
	ADD COLUMN kind   notification_template_kind DEFAULT 'system'::notification_template_kind NOT NULL;
COMMENT ON COLUMN notification_templates.method IS 'NULL defers to the deployment-level method';

-- No equivalent in down migration because ENUM values cannot be deleted.
ALTER TYPE notification_message_status ADD VALUE IF NOT EXISTS 'inhibited';

-- Function to prevent enqueuing notifications unnecessarily.
CREATE OR REPLACE FUNCTION inhibit_enqueue_if_disabled()
	RETURNS TRIGGER AS
$$
BEGIN
	-- Fail the insertion if the user has disabled this notification.
	IF EXISTS (SELECT 1
			   FROM notification_preferences
			   WHERE disabled = TRUE
				 AND user_id = NEW.user_id
				 AND notification_template_id = NEW.notification_template_id) THEN
		RAISE EXCEPTION 'cannot enqueue message: user has disabled this notification';
	END IF;

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to execute above function on insertion.
CREATE TRIGGER inhibit_enqueue_if_disabled
	BEFORE INSERT
	ON notification_messages
	FOR EACH ROW
EXECUTE FUNCTION inhibit_enqueue_if_disabled();

-- Allow modifications to notification templates to be audited.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'notification_template';
