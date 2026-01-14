# Workspace Runtime Audit

A SQL script that analyzes workspace builds to determine how long each workspace spent in a "running" state. It tracks state transitions (start/stop/delete) and calculates the cumulative runtime, only counting time spent inside the audit window period.

## Usage

1. Edit the date range in `workspace-runtime-audit.sql`:

```sql
start_time TIMESTAMPTZ := '2025-12-01 00:00:00+00';
end_time TIMESTAMPTZ := '2025-12-31 23:59:59+00';
```

2. Run against your Coder database:

```bash
psql -d coder -f scripts/workspace-runtime-audit/workspace-runtime-audit.sql
```

3. Review the output csv at `workspace_usage.csv`.

## Output

| Column                 | Type        | Description                                        |
|------------------------|-------------|----------------------------------------------------|
| `workspace_id`         | timestamptz | Name of the workspace                              |
| `workspace_created_at` | timestamptz | When the workspace was originally created          |
| `usage_hours`          | int         | Total number of usage hours within the time window |
