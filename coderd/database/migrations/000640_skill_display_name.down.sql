ALTER TABLE skills
    DROP CONSTRAINT skills_display_name_length,
    DROP CONSTRAINT skills_display_name_project_only,
    DROP COLUMN display_name;
