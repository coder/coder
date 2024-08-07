CREATE TABLE provisioner_job_timings
(
    provisioner_job_id uuid                     NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    started_at         timestamp with time zone not null,
    ended_at           timestamp with time zone not null,
    context            text                     not null, -- TODO: enum?
    action             text                     not null, -- TODO: enum?
    resource           text                     not null  -- TODO: enum?
);