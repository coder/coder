CREATE TYPE build_reason AS ENUM ('initiator', 'autostart', 'autostop');

ALTER TABLE ONLY workspace_builds
    ADD COLUMN IF NOT EXISTS reason build_reason NOT NULL DEFAULT 'initiator';
