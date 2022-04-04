ALTER TABLE ONLY workspaces
    ADD COLUMN autostart_schedule text DEFAULT NULL,
    ADD COLUMN autostop_schedule text DEFAULT NULL;
