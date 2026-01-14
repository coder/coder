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
	-- CHANGE THE START/END TIME HERE FOR YOUR AUDIT
	start_time TIMESTAMPTZ := '2025-12-01 00:00:00+00';
	end_time TIMESTAMPTZ := '2025-12-31 23:59:59+00';
	debug_mode BOOLEAN := FALSE;
	workspace RECORD;
	workspace_build RECORD;
	workspace_turned_on TIMESTAMPTZ;
	workspace_turned_off TIMESTAMPTZ;
	workspace_usage_duration INTERVAL;
	latest_build_created_at TIMESTAMPTZ;
	skipped_workspaces INTEGER := 0;
BEGIN
	FOR workspace IN
		SELECT
			workspaces.*
		FROM
		    workspaces
	LOOP
		-- Initialize variables for each workspace.
		workspace_turned_on = '0001-01-01 00:00:00+00';
		workspace_usage_duration = 0;
		IF debug_mode THEN
			RAISE NOTICE 'Processing Workspace ID: %, Created At: %', workspace.id, workspace.created_at;
		end if;

		latest_build_created_at := (
			SELECT wb.created_at
			FROM workspace_builds wb
			WHERE wb.workspace_id = workspace.id
			ORDER BY wb.build_number DESC
			LIMIT 1
		);
		IF latest_build_created_at < start_time THEN
			skipped_workspaces = skipped_workspaces + 1;
			CONTINUE;
		END IF;


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
					workspace_turned_off = COALESCE(workspace_build.completed_at, workspace_build.created_at);

					-- We only track usage within the start_time and end_time range.
					IF
						-- Turned off before the start time, so this workspace lived and died before our audit period.
						workspace_turned_off > start_time AND
						workspace_turned_on < end_time
					THEN
						IF workspace_turned_on < start_time THEN
							-- Started before the audit period, so move the turned_on to start_time.
							workspace_turned_on = start_time;
						END IF;

						IF workspace_turned_off > end_time THEN
							-- Turned off after the audit period, so move the turned_off to end_time.
							workspace_turned_off = end_time;
						END IF;
						workspace_usage_duration = workspace_usage_duration + (workspace_turned_off - workspace_turned_on);
						IF debug_mode THEN
							RAISE NOTICE 'Workspace (build %) turning OFF at % with % duration accumulated',
								workspace_build.build_number, workspace_turned_off, (workspace_turned_off - workspace_turned_on);
						END IF;
					ELSE
						IF debug_mode THEN
							RAISE NOTICE 'Workspace (build %) indicated activity outside the audit period, no duration accumulated',
								workspace_build.build_number;
						END IF;
					END IF;

					-- Always reset turned_on even if the duration was not accumulated.
					workspace_turned_on = '0001-01-01 00:00:00+00';
				END IF;
			END IF;
		END LOOP;

		IF workspace_turned_on != '0001-01-01 00:00:00+00' THEN
			IF workspace_turned_on < end_time THEN
				IF debug_mode THEN
					RAISE NOTICE 'Workspace still on at end of period, adding %', (end_time - workspace_turned_on);
				END IF;
				workspace_usage_duration = workspace_usage_duration + (end_time - workspace_turned_on);
			end if;
			workspace_turned_on = '0001-01-01 00:00:00+00';
		END IF;

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

		IF debug_mode THEN
			RAISE NOTICE 'Workspace ID: %, Created At: % has %d usage', workspace.id, workspace.created_at, EXTRACT(EPOCH FROM workspace_usage_duration) / 3600;
		end if;
	END LOOP;
	IF debug_mode THEN
		RAISE NOTICE 'Skipped % workspaces due to latest build before audit period', skipped_workspaces;
	END IF;
END$$ LANGUAGE plpgsql;

SELECT *
FROM _workspace_usage_results
ORDER BY usage_hours DESC, workspace_id;

COMMIT;
