ALTER TABLE ONLY workspaces
    ADD COLUMN IF NOT EXISTS autostart_schedule text DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS autostop_schedule text DEFAULT NULL;
