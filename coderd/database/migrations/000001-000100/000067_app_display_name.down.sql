-- Select all apps with an extra "row_number" column that determines the "rank"
-- of the display name against other display names in the same agent.
WITH row_numbers AS (
    SELECT
        *,
        row_number() OVER (PARTITION BY agent_id, display_name ORDER BY display_name ASC) AS row_number
    FROM
        workspace_apps
)

-- Update any app with a "row_number" greater than 1 to have the row number
-- appended to the display name. This effectively means that all lowercase
-- display names remain untouched, while non-unique mixed case usernames are
-- appended with a unique number. If you had three apps called all called asdf,
-- they would then be renamed to e.g. asdf, asdf1234, and asdf5678.
UPDATE
    workspace_apps
SET
    display_name = workspace_apps.display_name || floor(random() * 10000)::text
FROM
    row_numbers
WHERE
    workspace_apps.id = row_numbers.id AND
    row_numbers.row_number > 1;

-- rename column "display_name" to "name" on "workspace_apps"
ALTER TABLE "workspace_apps" RENAME COLUMN "display_name" TO "name";

-- restore unique index on "workspace_apps" table
ALTER TABLE workspace_apps ADD CONSTRAINT workspace_apps_agent_id_name_key UNIQUE ("agent_id", "name");
