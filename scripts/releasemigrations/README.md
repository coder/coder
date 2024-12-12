# Migration Releases

The `main.go` is a program that lists all releases and which migrations are contained with each upgrade.

## Usage

```shell
releasemigrations [--patches] [--minors] [--majors]
  -after-v2
        Only include releases after v2.0.0
  -dir string
        Migration directory (default "coderd/database/migrations")
  -list
        List migrations
  -majors
        Include major releases
  -minors
        Include minor releases
  -patches
        Include patches releases
  -versions string
        Comma separated list of versions to use. This skips uses git tag to find tags.
```

## Examples

### Find all migrations between 2 versions

Going from 2.3.0 to 2.4.0

```shell
$ go run scripts/releasemigrations/main.go --list --versions=v2.3.0,v2.4.0                                                                                                       11:47:00 AM
2023/11/21 11:47:09 [minor] 4 migrations added between v2.3.0 and v2.4.0
2023/11/21 11:47:09     coderd/database/migrations/000165_prevent_autostart_days.up.sql
2023/11/21 11:47:09     coderd/database/migrations/000166_template_active_version.up.sql
2023/11/21 11:47:09     coderd/database/migrations/000167_workspace_agent_api_version.up.sql
2023/11/21 11:47:09     coderd/database/migrations/000168_pg_coord_tailnet_v2_api.up.sql
2023/11/21 11:47:09 Patches: 0 (0 with migrations)
2023/11/21 11:47:09 Minors: 1 (1 with migrations)
2023/11/21 11:47:09 Majors: 0 (0 with migrations)
```

## Looking at all patch releases after v2

```shell
$ go run scripts/releasemigrations/main.go --patches --after-v2                                                                                                                  11:47:09 AM
2023/11/21 11:48:00 [patch] No migrations added between v2.0.0 and v2.0.1
2023/11/21 11:48:00 [patch] 2 migrations added between v2.0.1 and v2.0.2
2023/11/21 11:48:00 [patch] No migrations added between v2.1.0 and v2.1.1
2023/11/21 11:48:00 [patch] No migrations added between v2.1.1 and v2.1.2
2023/11/21 11:48:00 [patch] No migrations added between v2.1.2 and v2.1.3
2023/11/21 11:48:00 [patch] 1 migrations added between v2.1.3 and v2.1.4
2023/11/21 11:48:00 [patch] 2 migrations added between v2.1.4 and v2.1.5
2023/11/21 11:48:00 [patch] 1 migrations added between v2.3.0 and v2.3.1
2023/11/21 11:48:00 [patch] 1 migrations added between v2.3.1 and v2.3.2
2023/11/21 11:48:00 [patch] 1 migrations added between v2.3.2 and v2.3.3
2023/11/21 11:48:00 Patches: 10 (6 with migrations)
2023/11/21 11:48:00 Minors: 4 (4 with migrations)
2023/11/21 11:48:00 Majors: 0 (0 with migrations)
```

## Seeing all the noise this thing can make

This shows when every migration was introduced.

```shell
$ go run scripts/releasemigrations/main.go --patches --minors --majors --list
# ...
2023/11/21 11:48:31 [minor] 5 migrations added between v2.2.1 and v2.3.0
2023/11/21 11:48:31     coderd/database/migrations/000160_provisioner_job_status.up.sql
2023/11/21 11:48:31     coderd/database/migrations/000161_workspace_agent_stats_template_id_created_at_user_id_include_sessions.up.sql
2023/11/21 11:48:31     coderd/database/migrations/000162_workspace_automatic_updates.up.sql
2023/11/21 11:48:31     coderd/database/migrations/000163_external_auth_extra.up.sql
2023/11/21 11:48:31     coderd/database/migrations/000164_archive_template_versions.up.sql
2023/11/21 11:48:31 [patch] 1 migrations added between v2.3.0 and v2.3.1
2023/11/21 11:48:31     coderd/database/migrations/000165_prevent_autostart_days.up.sql
2023/11/21 11:48:31 [patch] 1 migrations added between v2.3.1 and v2.3.2
2023/11/21 11:48:31     coderd/database/migrations/000166_template_active_version.up.sql
2023/11/21 11:48:31 [patch] 1 migrations added between v2.3.2 and v2.3.3
2023/11/21 11:48:31     coderd/database/migrations/000167_workspace_agent_api_version.up.sql
2023/11/21 11:48:31 [minor] 1 migrations added between v2.3.3 and v2.4.0
2023/11/21 11:48:31     coderd/database/migrations/000168_pg_coord_tailnet_v2_api.up.sql
2023/11/21 11:48:31 Patches: 122 (55 with migrations)
2023/11/21 11:48:31 Minors: 31 (26 with migrations)
2023/11/21 11:48:31 Majors: 1 (1 with migrations)
```
