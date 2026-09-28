-- Drop views before modifying underlying tables and columns.
DROP VIEW IF EXISTS workspaces_expanded;

-- Recreate task status enum type.
CREATE TYPE task_status AS ENUM (
    'pending',
    'initializing',
    'active',
    'paused',
    'unknown',
    'error'
);

-- Recreate tasks table. Deleted task rows cannot be recovered.
CREATE TABLE tasks (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    name text NOT NULL,
    workspace_id uuid,
    template_version_id uuid NOT NULL,
    template_parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    prompt text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone,
    display_name character varying(127) DEFAULT ''::character varying NOT NULL
);

COMMENT ON COLUMN tasks.display_name IS 'Display name is a custom, human-friendly task name.';

ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

CREATE INDEX tasks_organization_id_idx ON tasks USING btree (organization_id);
CREATE INDEX tasks_owner_id_idx ON tasks USING btree (owner_id);
CREATE UNIQUE INDEX tasks_owner_id_name_unique_idx ON tasks USING btree (owner_id, lower(name)) WHERE (deleted_at IS NULL);
COMMENT ON INDEX tasks_owner_id_name_unique_idx IS 'Index to ensure uniqueness for task owner/name';
CREATE INDEX tasks_workspace_id_idx ON tasks USING btree (workspace_id);

ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_template_version_id_fkey FOREIGN KEY (template_version_id) REFERENCES template_versions(id) ON DELETE CASCADE;
ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE;

-- Recreate task_workspace_apps table.
CREATE TABLE task_workspace_apps (
    task_id uuid NOT NULL,
    workspace_agent_id uuid,
    workspace_app_id uuid,
    workspace_build_number integer NOT NULL
);

ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_pkey PRIMARY KEY (task_id, workspace_build_number);

ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;
ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_workspace_agent_id_fkey FOREIGN KEY (workspace_agent_id) REFERENCES workspace_agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY task_workspace_apps
    ADD CONSTRAINT task_workspace_apps_workspace_app_id_fkey FOREIGN KEY (workspace_app_id) REFERENCES workspace_apps(id) ON DELETE CASCADE;

-- Recreate task_snapshots table.
CREATE TABLE task_snapshots (
    task_id uuid NOT NULL,
    log_snapshot jsonb NOT NULL,
    log_snapshot_created_at timestamp with time zone DEFAULT now() NOT NULL
);

COMMENT ON TABLE task_snapshots IS 'Stores snapshots of task state when paused, currently limited to conversation history.';
COMMENT ON COLUMN task_snapshots.task_id IS 'The task this snapshot belongs to.';
COMMENT ON COLUMN task_snapshots.log_snapshot IS 'Task conversation history in JSON format, allowing users to view logs when the workspace is stopped.';
COMMENT ON COLUMN task_snapshots.log_snapshot_created_at IS 'When this log snapshot was captured.';

ALTER TABLE ONLY task_snapshots
    ADD CONSTRAINT task_snapshots_pkey PRIMARY KEY (task_id);

ALTER TABLE ONLY task_snapshots
    ADD CONSTRAINT task_snapshots_task_id_fkey FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;

-- Recreate views.
CREATE VIEW workspaces_expanded AS
 SELECT workspaces.id,
    workspaces.created_at,
    workspaces.updated_at,
    workspaces.owner_id,
    workspaces.organization_id,
    workspaces.template_id,
    workspaces.deleted,
    workspaces.name,
    workspaces.autostart_schedule,
    workspaces.ttl,
    workspaces.last_used_at,
    workspaces.dormant_at,
    workspaces.deleting_at,
    workspaces.automatic_updates,
    workspaces.favorite,
    workspaces.next_start_at,
    workspaces.group_acl,
    workspaces.user_acl,
    visible_users.avatar_url AS owner_avatar_url,
    visible_users.username AS owner_username,
    visible_users.name AS owner_name,
    organizations.name AS organization_name,
    organizations.display_name AS organization_display_name,
    organizations.icon AS organization_icon,
    organizations.description AS organization_description,
    templates.name AS template_name,
    templates.display_name AS template_display_name,
    templates.icon AS template_icon,
    templates.description AS template_description,
    tasks.id AS task_id,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(g.name, ''::text), 'avatar_url', COALESCE(g.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.group_acl) acl(key, value)
             LEFT JOIN groups g ON ((g.id = (acl.key)::uuid)))), '{}'::jsonb) AS group_acl_display_info,
    COALESCE(( SELECT jsonb_object_agg(acl.key, jsonb_build_object('name', COALESCE(vu.name, ''::text), 'avatar_url', COALESCE(vu.avatar_url, ''::text))) AS jsonb_object_agg
           FROM (jsonb_each(workspaces.user_acl) acl(key, value)
             LEFT JOIN visible_users vu ON ((vu.id = (acl.key)::uuid)))), '{}'::jsonb) AS user_acl_display_info
   FROM ((((workspaces
     JOIN visible_users ON ((workspaces.owner_id = visible_users.id)))
     JOIN organizations ON ((workspaces.organization_id = organizations.id)))
     JOIN templates ON ((workspaces.template_id = templates.id)))
     LEFT JOIN tasks ON ((workspaces.id = tasks.workspace_id)));

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';

CREATE VIEW tasks_with_status AS
 SELECT tasks.id,
    tasks.organization_id,
    tasks.owner_id,
    tasks.name,
    tasks.workspace_id,
    tasks.template_version_id,
    tasks.template_parameters,
    tasks.prompt,
    tasks.created_at,
    tasks.deleted_at,
    tasks.display_name,
    COALESCE(workspaces.group_acl, '{}'::jsonb) AS workspace_group_acl,
    COALESCE(workspaces.user_acl, '{}'::jsonb) AS workspace_user_acl,
        CASE
            WHEN (tasks.workspace_id IS NULL) THEN 'pending'::task_status
            WHEN (build_status.status <> 'active'::task_status) THEN build_status.status
            WHEN (agent_status.status <> 'active'::task_status) THEN agent_status.status
            ELSE app_status.status
        END AS status,
    jsonb_build_object('build', jsonb_build_object('transition', latest_build_raw.transition, 'job_status', latest_build_raw.job_status, 'computed', build_status.status), 'agent', jsonb_build_object('lifecycle_state', agent_raw.lifecycle_state, 'computed', agent_status.status), 'app', jsonb_build_object('health', app_raw.health, 'computed', app_status.status)) AS status_debug,
    task_app.workspace_build_number,
    task_app.workspace_agent_id,
    task_app.workspace_app_id,
    agent_raw.lifecycle_state AS workspace_agent_lifecycle_state,
    app_raw.health AS workspace_app_health,
    task_owner.owner_username,
    task_owner.owner_name,
    task_owner.owner_avatar_url
   FROM (((((((((tasks
     LEFT JOIN workspaces ON ((workspaces.id = tasks.workspace_id)))
     CROSS JOIN LATERAL ( SELECT vu.username AS owner_username,
            vu.name AS owner_name,
            vu.avatar_url AS owner_avatar_url
           FROM visible_users vu
          WHERE (vu.id = tasks.owner_id)) task_owner)
     LEFT JOIN LATERAL ( SELECT task_app_1.workspace_build_number,
            task_app_1.workspace_agent_id,
            task_app_1.workspace_app_id
           FROM task_workspace_apps task_app_1
          WHERE (task_app_1.task_id = tasks.id)
          ORDER BY task_app_1.workspace_build_number DESC
         LIMIT 1) task_app ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_build.transition,
            provisioner_job.job_status,
            workspace_build.job_id
           FROM (workspace_builds workspace_build
             JOIN provisioner_jobs provisioner_job ON ((provisioner_job.id = workspace_build.job_id)))
          WHERE ((workspace_build.workspace_id = tasks.workspace_id) AND (workspace_build.build_number = task_app.workspace_build_number))) latest_build_raw ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_agent.lifecycle_state
           FROM workspace_agents workspace_agent
          WHERE (workspace_agent.id = task_app.workspace_agent_id)) agent_raw ON (true))
     LEFT JOIN LATERAL ( SELECT workspace_app.health
           FROM workspace_apps workspace_app
          WHERE (workspace_app.id = task_app.workspace_app_id)) app_raw ON (true))
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN (latest_build_raw.job_status IS NULL) THEN 'pending'::task_status
                    WHEN (latest_build_raw.job_status = ANY (ARRAY['failed'::provisioner_job_status, 'canceling'::provisioner_job_status, 'canceled'::provisioner_job_status])) THEN 'error'::task_status
                    WHEN ((latest_build_raw.transition = ANY (ARRAY['stop'::workspace_transition, 'delete'::workspace_transition])) AND (latest_build_raw.job_status = 'succeeded'::provisioner_job_status)) THEN 'paused'::task_status
                    WHEN ((latest_build_raw.transition = 'start'::workspace_transition) AND (latest_build_raw.job_status = 'pending'::provisioner_job_status)) THEN 'pending'::task_status
                    WHEN ((latest_build_raw.transition = 'start'::workspace_transition) AND (latest_build_raw.job_status = ANY (ARRAY['running'::provisioner_job_status, 'succeeded'::provisioner_job_status]))) THEN 'active'::task_status
                    ELSE 'unknown'::task_status
                END AS status) build_status)
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN ((agent_raw.lifecycle_state IS NULL) OR (agent_raw.lifecycle_state = ANY (ARRAY['created'::workspace_agent_lifecycle_state, 'starting'::workspace_agent_lifecycle_state]))) THEN 'initializing'::task_status
                    WHEN (agent_raw.lifecycle_state = ANY (ARRAY['ready'::workspace_agent_lifecycle_state, 'start_timeout'::workspace_agent_lifecycle_state, 'start_error'::workspace_agent_lifecycle_state])) THEN 'active'::task_status
                    WHEN (agent_raw.lifecycle_state <> ALL (ARRAY['created'::workspace_agent_lifecycle_state, 'starting'::workspace_agent_lifecycle_state, 'ready'::workspace_agent_lifecycle_state, 'start_timeout'::workspace_agent_lifecycle_state, 'start_error'::workspace_agent_lifecycle_state])) THEN 'unknown'::task_status
                    ELSE 'unknown'::task_status
                END AS status) agent_status)
     CROSS JOIN LATERAL ( SELECT
                CASE
                    WHEN (app_raw.health = 'initializing'::workspace_app_health) THEN 'initializing'::task_status
                    WHEN (app_raw.health = 'unhealthy'::workspace_app_health) THEN 'error'::task_status
                    WHEN (app_raw.health = ANY (ARRAY['healthy'::workspace_app_health, 'disabled'::workspace_app_health])) THEN 'active'::task_status
                    ELSE 'unknown'::task_status
                END AS status) app_status)
  WHERE (tasks.deleted_at IS NULL);
