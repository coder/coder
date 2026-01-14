-- SELECT * FROM workspaces WHERE id = 'f5bd1e71-effd-4dee-9fe7-1fc3a481eaee';
-- SELECT workspace_builds.build_number, transition, * FROM workspace_builds WHERE workspace_id = 'f5bd1e71-effd-4dee-9fe7-1fc3a481eaee' ORDER BY build_number ASC;
-- SELECT * FROM users WHERE id = '0bac0dfd-b086-4b6d-b8ba-789e0eca7451';

-- Workspace Runtime Audit Report
--
-- This script calculates total workspace runtime within a specified date range.
-- It tracks workspace state transitions (start/stop/delete) and sums up the time
-- each workspace spent in a "running" state.
--
-- Usage:
--   1. Edit the start_time and end_time in the params CTE below
--   2. Run: psql -f workspace-runtime-audit.sql

BEGIN;
-- 1) Temp table to hold the data aggregated from the anonymous function
CREATE TEMP TABLE _workspace_usage_results (
   workspace_id uuid,
   workspace_created_at timestamptz,
   usage_hours INTEGER
) ON COMMIT DROP;

DO $$
DECLARE
	start_time TIMESTAMPTZ := '2025-12-01 00:00:00+00';
	end_time TIMESTAMPTZ := '2025-12-31 23:59:59+00';
	debug_mode BOOLEAN := TRUE;
	workspace RECORD;
	workspace_build RECORD;
	workspace_turned_on TIMESTAMPTZ;
	workspace_usage_duration INTERVAL;
BEGIN
	FOR workspace IN SELECT	* FROM workspaces WHERE id = 'f5bd1e71-effd-4dee-9fe7-1fc3a481eaee'
	LOOP
		-- Initialize variables for each workspace.
		workspace_turned_on = '0001-01-01 00:00:00+00';
		workspace_usage_duration = 0;
		IF debug_mode THEN
			RAISE NOTICE 'Processing Workspace ID: %, Created At: %', workspace.id, workspace.created_at;
		end if;

		-- For every workspace, calculate the total runtime within the specified date range.
		FOR workspace_build IN
			SELECT
				workspace_builds.*,
				provisioner_jobs.job_status,
				provisioner_jobs.completed_at
			FROM workspace_builds
				LEFT JOIN provisioner_jobs ON workspace_builds.job_id = provisioner_jobs.id
			    WHERE workspace_id = workspace.id
			    ORDER BY workspace_builds.build_number ASC
		LOOP
			-- Initialize last_transition state for duration accumulation.

-- 			IF workspace_build.build_number = 1 THEN
-- 				workspace_turned_on = workspace_build.created_at;
-- 			END IF;

			-- Usage only counts from workspaces that successfully started
			IF workspace_build.transition = 'start' AND
			   workspace_build.job_status IN ('succeeded') THEN

			    -- If the workspace is already turned on (e.g., multiple starts without stops),
			    -- we consider the previous start time as usage time. So ignore this start.
			    IF workspace_turned_on = '0001-01-01 00:00:00+00' THEN
					workspace_turned_on = COALESCE(workspace_build.completed_at, workspace_build.created_at);
					IF debug_mode THEN
						RAISE NOTICE 'Workspace (build %) turned ON at %', workspace_build.build_number, workspace_turned_on;
					END IF;
				END IF;
			ELSE
				-- All other transitions and job status are treated as a workspace stopping.
				-- Only accumulate time from the last successful start to this point.
				IF workspace_turned_on != '0001-01-01 00:00:00+00' THEN
					IF debug_mode THEN
						RAISE NOTICE 'Workspace (build %) turning OFF at % with % duration accumulated',
							workspace_build.build_number, COALESCE(workspace_build.created_at, workspace_build.completed_at), (COALESCE(workspace_build.created_at) - workspace_turned_on);
					END IF;
					workspace_usage_duration = workspace_usage_duration + (COALESCE(workspace_build.created_at) - workspace_turned_on);
					workspace_turned_on = '0001-01-01 00:00:00+00'; -- Reset turned_on since workspace is now considerd "off".
				END IF;
			END IF;
		END LOOP;

		IF workspace_turned_on != '0001-01-01 00:00:00+00' THEN
			-- Workspace is still on at the end of the audit period, accumulate time until end_time.
			RAISE NOTICE 'Workspace still on at end of period, adding %', (end_time - workspace_turned_on);
			workspace_usage_duration = workspace_usage_duration + (end_time - workspace_turned_on);
		END IF;

		-- This is the "last RAISE NOTICE" converted into a row insert
		INSERT INTO _workspace_usage_results (
			workspace_id,
			workspace_created_at,
			usage_hours
		)
		VALUES (
		   workspace.id,
		   workspace.created_at,
		   (EXTRACT(EPOCH FROM workspace_usage_duration) / 3600.0)
	   	);

		RAISE NOTICE 'Workspace ID: %, Created At: % has %d usage', workspace.id, workspace.created_at, EXTRACT(EPOCH FROM workspace_usage_duration) / 3600;
	END LOOP;
END$$ LANGUAGE plpgsql;

SELECT *
FROM _workspace_usage_results
ORDER BY usage_hours DESC, workspace_id;

COMMIT;
