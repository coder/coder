-- Workspace Runtime Audit Report
-- !! CAUTION: Do not run this script unless specifically instructed to do so by Coder support or engineering.
--
-- This script calculates total workspace runtime within a specified date range.
-- It tracks workspace state transitions (start/stop/delete) and sums up the time
-- each workspace spent in a "running" state. A "running" state is considered to be
-- any workspace that has been started successfully and not yet stopped, deleted, or failed.
--
-- All usage (per workspace) is rounded up to the nearest hour. So usage is accumulated for the time period
-- per workspace, then rounded up to the next hour.
-- So a workspace that runs for 40s, will be 1 hour of usage.
-- If a workspace runs for 40s, then stops, then runs for another 40s will still be 1 hour.
--
-- Usage:
--   1. Edit the start_time and end_time in the params below
--   2. Run: psql -f workspace-runtime-audit.sql
--   3. A file called 'workspace_usage.csv' will be generated with the results.
BEGIN;
-- Temp table to hold the data aggregated from the anonymous function.
-- It is dropped automatically at the end of the transaction.
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
	-- 'debug_mode' emits logging to help trace processing for each workspace
	debug_mode BOOLEAN := FALSE;
	-- temporary variables
	workspace RECORD;
	workspace_build RECORD;
	workspace_turned_on TIMESTAMPTZ;
	workspace_turned_off TIMESTAMPTZ;
	workspace_usage_duration INTERVAL;
	latest_build_created_at TIMESTAMPTZ;
	latest_build_transition workspace_transition;
	-- Counter for skipped workspaces that are outside the window, used for debugging
	skipped_workspaces INTEGER := 0;
BEGIN
	FOR workspace IN
		SELECT
			workspaces.*
		FROM
		    workspaces
		-- Adding a WHERE clause here to limit the workspaces is helpful during testing.
	LOOP
		-- Initialize variables for each workspace and prevent variable carry-over between workspaces.
		workspace_turned_on = '0001-01-01 00:00:00+00';
		workspace_usage_duration = 0;
		IF debug_mode THEN
			RAISE NOTICE 'Processing Workspace ID: %, Created At: %', workspace.id, workspace.created_at;
		end if;

		-- Fetch the latest build for the workspace to determine if we can skip it entirely.
		SELECT
			wb.created_at, wb.transition
		FROM
			workspace_builds wb
		WHERE
			wb.workspace_id = workspace.id
		ORDER BY wb.build_number DESC
		LIMIT 1
		INTO
			latest_build_created_at,
			latest_build_transition
		;

		-- If the latest build is a stop/delete before our start time, this workspace had no activity in our audit period.
		-- This is an optimization to skip processing workspaces that are clearly before the audit period. Workspaces created
		-- after the audit period are still processed. Since this is expected to be run periodically, optimizing out old workspaces
		-- is likely to be more beneficial than future workspaces.
		IF
			latest_build_created_at < start_time AND latest_build_transition != 'start'
		THEN
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
			-- Algorithm summary:
			-- LOOP:
			--   1. When a successful start is found, set 'workspace_turned_on'
			--     1a. If already turned on, ignore (multiple starts without stops)
			--   2. Any other transition (stop/delete/failed) will calculate the duration from 'workspace_turned_on' to this point
			-- 3. After the loop, if still turned on, calculate duration from 'workspace_turned_on' to 'end_time'

			-- Usage only counts from workspaces that successfully started
			IF workspace_build.transition = 'start' AND
			   workspace_build.job_status IN ('succeeded')
			THEN
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
				-- You could imagine a workspace that was only failing builds and never accumulating time.
				IF workspace_turned_on != '0001-01-01 00:00:00+00'
				THEN
					workspace_turned_off = COALESCE(workspace_build.completed_at, workspace_build.created_at);
					-- We only track usage within the start_time and end_time range.
					IF
						-- Turned off before the start time, so this workspace lived and died before our audit period.
						workspace_turned_off > start_time AND
						-- Turned on before the end time. If this is in the future, then it didn't run during our audit period.
						workspace_turned_on < end_time
					THEN
						-- Fix the on/off times to be within the audit period. Ignore any time outside the period.
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

		-- After processing all builds, if the workspace is still turned on, accumulate time until end_time.
		-- This handles workspaces that were started but never stopped/deleted.
		IF workspace_turned_on != '0001-01-01 00:00:00+00' THEN
			IF workspace_turned_on < start_time THEN
				workspace_turned_on = start_time;
			END IF;
			IF workspace_turned_on < end_time THEN
				IF debug_mode THEN
					RAISE NOTICE 'Workspace still on at end of period, adding %', (end_time - workspace_turned_on);
				END IF;
				workspace_usage_duration = workspace_usage_duration + (end_time - workspace_turned_on);
			END IF;
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
		   -- Only tracking whole hours for simplicity. Always rounding up to the next hour.
		   CEIL((EXTRACT(EPOCH FROM workspace_usage_duration) / 3600.0))
	   	);

		IF debug_mode THEN
			RAISE NOTICE 'Workspace ID: %, Created At: % has %d usage', workspace.id, workspace.created_at, EXTRACT(EPOCH FROM workspace_usage_duration) / 3600;
		end if;
	END LOOP;
	IF debug_mode THEN
		RAISE NOTICE 'Skipped % workspaces due to latest build before audit period', skipped_workspaces;
	END IF;
END$$ LANGUAGE plpgsql;

-- Export the results to a CSV file
\copy (SELECT * FROM _workspace_usage_results WHERE usage_hours > 0 ORDER BY usage_hours DESC) TO 'workspace_usage.csv' WITH (FORMAT CSV, HEADER TRUE);

-- Optionally use a select to view results as a table output
-- SELECT * FROM _workspace_usage_results WHERE usage_hours > 0 ORDER BY usage_hours DESC;

COMMIT;
