-- Initialize PostgreSQL database for ci report analysis

CREATE TABLE IF NOT EXISTS tests (
	id BIGSERIAL PRIMARY KEY,
	package TEXT NOT NULL,
	name TEXT,
	added TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(package, name)
);

COMMENT ON TABLE tests IS 'List of all unique tests that have been run, including packages and package tests';
COMMENT ON COLUMN tests.package IS 'The package that was tested or the package that the test belongs to';
COMMENT ON COLUMN tests.name IS 'The name of the test, omitted if package';
COMMENT ON COLUMN tests.added IS 'When the test was first seen';
COMMENT ON COLUMN tests.last_seen IS 'When the test was last seen';

CREATE TABLE IF NOT EXISTS runs (
	id BIGSERIAL PRIMARY KEY,
	run_id BIGINT NOT NULL,
	event TEXT NOT NULL,
	branch TEXT NOT NULL,
	commit TEXT NOT NULL,
	commit_message TEXT NOT NULL,
	author TEXT NOT NULL,
	ts TIMESTAMPTZ NOT NULL,
	UNIQUE(run_id)
);

COMMENT ON TABLE runs IS 'List of all runs that have been run';
COMMENT ON COLUMN runs.run_id IS 'The unique ID of the run (from CI)';
COMMENT ON COLUMN runs.event IS 'The type of event that triggered the run';
COMMENT ON COLUMN runs.branch IS 'The branch that the run was triggered on';
COMMENT ON COLUMN runs.commit IS 'The commit that the run was triggered on';
COMMENT ON COLUMN runs.commit_message IS 'The commit message that the run was triggered on';
COMMENT ON COLUMN runs.author IS 'The author of the commit that the run was triggered on';
COMMENT ON COLUMN runs.ts IS 'The date/time that the workflow was run';

CREATE TABLE IF NOT EXISTS jobs (
	id BIGSERIAL PRIMARY KEY,
	run_id BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE ON UPDATE CASCADE,
	job_id BIGINT NOT NULL,
	name TEXT NOT NULL,
	ts TIMESTAMPTZ NOT NULL,
	UNIQUE(run_id, job_id)
);

COMMENT ON TABLE jobs IS 'List of all jobs that have been run (a job is e.g. test-go (ubuntu-latest))';
COMMENT ON COLUMN jobs.run_id IS 'The run that the job belongs to';
COMMENT ON COLUMN jobs.job_id IS 'The unique ID of the job (from CI)';
COMMENT ON COLUMN jobs.name IS 'The name of the job';
COMMENT ON COLUMN jobs.ts IS 'The date/time that the job was run';

DO $$ BEGIN
IF to_regtype('test_status') IS NULL THEN
	CREATE TYPE test_status AS ENUM ('pass', 'fail');
END IF;
END $$;

CREATE TABLE IF NOT EXISTS job_results (
	id BIGSERIAL PRIMARY KEY,
	job_id BIGINT NOT NULL REFERENCES jobs (id) ON DELETE CASCADE ON UPDATE CASCADE,
	test_id BIGINT NOT NULL REFERENCES tests (id) ON DELETE CASCADE ON UPDATE CASCADE,
	status test_status NOT NULL,
	timeout BOOLEAN NOT NULL,
	execution_time FLOAT,
	output TEXT,
	UNIQUE(job_id, test_id)
);

COMMENT ON TABLE job_results IS 'List of all results';
COMMENT ON COLUMN job_results.job_id IS 'The job that the result belongs to';
COMMENT ON COLUMN job_results.test_id IS 'The test that the result belongs to';
COMMENT ON COLUMN job_results.status IS 'The status of the test';
COMMENT ON COLUMN job_results.execution_time IS 'The execution time of the test';
COMMENT ON COLUMN job_results.output IS 'The output of the test (if failed)';
