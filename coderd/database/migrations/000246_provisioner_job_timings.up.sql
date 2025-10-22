CREATE TYPE provisioner_job_timing_stage AS ENUM (
    'init',
    'plan',
    'graph',
    'apply'
    );

CREATE TABLE provisioner_job_timings
(
    job_id     uuid                         NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    started_at timestamp with time zone     not null,
    ended_at   timestamp with time zone     not null,
    stage      provisioner_job_timing_stage not null,
    source     text                         not null,
    action     text                         not null,
    resource   text                         not null
);

CREATE VIEW provisioner_job_stats AS
SELECT pj.id                                                                                              AS job_id,
       pj.job_status,
       wb.workspace_id,
       pj.worker_id,
       pj.error,
       pj.error_code,
       pj.updated_at,
       GREATEST(EXTRACT(EPOCH FROM (pj.started_at - pj.created_at)), 0)                                   AS queued_secs,
       GREATEST(EXTRACT(EPOCH FROM (pj.completed_at - pj.started_at)), 0)                                 AS completion_secs,
       GREATEST(EXTRACT(EPOCH FROM (pj.canceled_at - pj.started_at)), 0)                                  AS canceled_secs,
       GREATEST(EXTRACT(EPOCH FROM (
           MAX(CASE WHEN pjt.stage = 'init'::provisioner_job_timing_stage THEN pjt.ended_at END) -
           MIN(CASE WHEN pjt.stage = 'init'::provisioner_job_timing_stage THEN pjt.started_at END))), 0)  AS init_secs,
       GREATEST(EXTRACT(EPOCH FROM (
           MAX(CASE WHEN pjt.stage = 'plan'::provisioner_job_timing_stage THEN pjt.ended_at END) -
           MIN(CASE WHEN pjt.stage = 'plan'::provisioner_job_timing_stage THEN pjt.started_at END))), 0)  AS plan_secs,
       GREATEST(EXTRACT(EPOCH FROM (
           MAX(CASE WHEN pjt.stage = 'graph'::provisioner_job_timing_stage THEN pjt.ended_at END) -
           MIN(CASE WHEN pjt.stage = 'graph'::provisioner_job_timing_stage THEN pjt.started_at END))), 0) AS graph_secs,
       GREATEST(EXTRACT(EPOCH FROM (
           MAX(CASE WHEN pjt.stage = 'apply'::provisioner_job_timing_stage THEN pjt.ended_at END) -
           MIN(CASE WHEN pjt.stage = 'apply'::provisioner_job_timing_stage THEN pjt.started_at END))), 0) AS apply_secs
FROM provisioner_jobs pj
         JOIN workspace_builds wb ON wb.job_id = pj.id
         LEFT JOIN provisioner_job_timings pjt ON pjt.job_id = pj.id
GROUP BY pj.id, wb.workspace_id;
