ALTER TABLE ONLY workspaces
    DROP COLUMN IF EXISTS autostart_schedule,
    DROP COLUMN IF EXISTS autostop_schedule;
