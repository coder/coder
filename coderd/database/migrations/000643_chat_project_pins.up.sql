CREATE TYPE chat_project_pin_source AS ENUM (
    'migration',
    'default',
    'user'
);

CREATE TABLE chat_project_pins (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    pin_order integer NOT NULL DEFAULT 0,
    source chat_project_pin_source NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    unpinned_at timestamptz,
    PRIMARY KEY (user_id, project_id),
    CONSTRAINT chat_project_pins_pin_order_check CHECK (
        (unpinned_at IS NULL AND pin_order > 0)
        OR (unpinned_at IS NOT NULL AND pin_order = 0)
    )
);

COMMENT ON TABLE chat_project_pins IS 'Per-user pinned chat projects. Pins can outlive project access, so readers must check access.';
COMMENT ON COLUMN chat_project_pins.unpinned_at IS 'When the user unpinned the project. A row with this set is a tombstone kept for telemetry, not a pin.';

CREATE INDEX idx_chat_project_pins_user_order ON chat_project_pins (user_id, pin_order) WHERE unpinned_at IS NULL;

-- Pin every project its owner can already see in the sidebar, in the
-- sidebar's order, so nothing disappears on upgrade. Shared projects are
-- not pinned.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT
    owner_id,
    id,
    ROW_NUMBER() OVER (PARTITION BY owner_id ORDER BY lower(name), id),
    'migration'
FROM chat_projects
ON CONFLICT DO NOTHING;
