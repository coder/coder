CREATE TABLE user_status_changes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    new_status user_status NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_status_changes IS 'Tracks the history of user status changes';

CREATE INDEX idx_user_status_changes_changed_at ON user_status_changes(changed_at);

INSERT INTO user_status_changes (
    user_id,
    new_status,
    changed_at
)
SELECT
    id,
    status,
    created_at
FROM users
WHERE NOT deleted;

CREATE OR REPLACE FUNCTION record_user_status_change() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM NEW.status THEN
        INSERT INTO user_status_changes (
            user_id,
            new_status,
            changed_at
        ) VALUES (
            NEW.id,
            NEW.status,
            NEW.updated_at
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER user_status_change_trigger
    AFTER INSERT OR UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION record_user_status_change();
