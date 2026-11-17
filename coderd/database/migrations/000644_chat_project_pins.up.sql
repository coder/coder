CREATE TYPE chat_project_pin_source AS ENUM (
    'migration',
    'default',
    'user'
);

CREATE TABLE chat_project_pins (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    pin_order integer NOT NULL CONSTRAINT chat_project_pins_pin_order_check CHECK (pin_order > 0),
    source chat_project_pin_source NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id)
);

COMMENT ON TABLE chat_project_pins IS 'Per-user pinned chat projects. Pins can outlive project access, so readers must check access.';

CREATE INDEX idx_chat_project_pins_user_order ON chat_project_pins (user_id, pin_order);

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
