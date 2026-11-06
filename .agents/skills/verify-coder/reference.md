# Verify Coder reference

Details behind the steps in `SKILL.md`. Paths are relative to this skill's directory unless they start with `build/` or `.coderv2/`, which are relative to the repo root.

## Launch

- `launch.sh` takes only `--fresh` and, after `--`, the coder server flags in `server_flag_allowed` (`--verbose`, `--experiments=<list>`). The harness owns the database, the ports, the addresses and the admin password, so it refuses every other argument.
- It builds `build/.bin/develop` and `build/coder_<os>_<arch>` first, so build errors land in `build/verify-coder/run/build.log`. The first build in a checkout compiles the site and seven slim binaries, which can take more than 10 minutes.
- It starts develop with `--port-offset`, develop's per-checkout offset (`.claude/docs/DEV_ISOLATION.md`), and reads the ports from develop's `resolved ports` log line. `CODER_DEV_PORT` and `CODER_DEV_WEB_PORT` set explicit ports instead.
- It starts develop through `vc_start_develop`, which removes every `CODER_*` variable of the calling shell, `BASH_ENV`, `ENV` and the `ANTHROPIC_*` and `OPENAI_*` keys. A `CODER_PG_CONNECTION_URL` or `CODER_DEV_DB_RESET` in your shell therefore never reaches the instance, and nothing spends real tokens.
- It holds `build/verify-coder/launch.lock`, so a second `launch.sh` in the same checkout stops at once. It refuses to start while `build/verify-coder/run/env` records an instance.
- Ready means develop printed `Coder is now running in development mode` in `$VC_LOG`. Then `launch.sh` checks the admin password and runs the doctor.
- `launch.sh` refuses `CODER_BUILD_AGPL` set to anything but empty, `0` or `false`, because `make` would reuse an existing enterprise binary.
- It sets `umask 022` for the build and the server. Some agent shells default to 077.

## Admin password

- `admin@coder.com` (owner) and `member@coder.com` share one password. `vc_load_run` sets `VC_ADMIN_PASSWORD` to it. Never print it.
- A database that the launch creates (`--fresh`, or no `.coderv2/postgres/data` yet) gets a random password, saved in `.coderv2/.verify-coder-admin-password` (mode 0600) once develop creates the first user. `CODER_DEV_ADMIN_PASSWORD` overrides it.
- Reused data uses `CODER_DEV_ADMIN_PASSWORD`, then that file. With neither, `launch.sh` refuses to start.
- It never runs with the published default `SomeSecurePassword!`. It refuses that password up front, and stops the server if the instance still accepts it once ready. Data from a plain `develop.sh` run has that default, so launch it with `--fresh`.
- After `--fresh` on your own `.coderv2`, a plain `scripts/develop.sh` run needs `CODER_DEV_ADMIN_PASSWORD="$(cat .coderv2/.verify-coder-admin-password)"` to sign in.

## State

- `build/verify-coder/run/` holds the env file, `build.log`, `develop.log`, journey pid files and the recorded workspaces. `.coderv2/` holds the built-in Postgres and the CLI session. Both are per checkout and gitignored.
- `launch.sh` makes `build/verify-coder/` mode 0700, because `develop.log` records the admin password. `VERIFY_CODER_STATE` moves that folder. Point it only at a folder the harness owns: cleanup deletes its `run/` subfolder.
- A second checkout can run its own instance at the same time. Two instances in one checkout cannot, because they share `.coderv2`.

## Doctor

`doctor.sh` checks that:

- the recorded pid is this checkout's `build/.bin/develop` session leader, and both ports belong to that session;
- `/healthz` answers and the `/api/v2/buildinfo` version (`-devel+<short sha>`) is `HEAD`. It compares commits only, so relaunch after uncommitted edits too;
- the CLI session in `.coderv2/session` is the admin;
- the API port serves the embedded frontend and the Vite port proxies `/api`;
- the `docker` template exists whenever `docker info` works;
- `build/verify-coder/` is mode 0700 and the published default password is refused.

It changes nothing apart from that one refused sign-in.

## Self-test

`scripts/selftest.sh` needs no instance and takes about a second. Run it after you edit `launch.sh`, `lib.sh` or `cleanup.sh`. It checks:

- the `launch.sh` refusals: `CODER_BUILD_AGPL`, develop.sh flags, server flags outside `server_flag_allowed` and the published default password;
- that a new database without `CODER_DEV_ADMIN_PASSWORD`, and the allowed server flags, get past those checks;
- the launch lock;
- that `vc_start_develop` passes the password through the environment and drops the caller's `CODER_*` variables and `BASH_ENV`;
- that a Docker error fails cleanup's leftover inventory;
- that `vc_browser_login` passes the password to agent-browser on stdin.

## Drive

- Web UI: `$VC_WEB_URL` is the production frontend that coder server serves from the built binary. Launch rebuilds it when a `.ts` or `.tsx` file under `site/` changed, so relaunch after a frontend edit. A change to only CSS or static files does not trigger the rebuild: touch a `.tsx` file or run `make -B site/out/index.html` first.
- `$VC_VITE_URL` is the Vite dev server with hot reload. Use it only to watch an edit live: under host load it failed lazy route imports (`Failed to fetch dynamically imported module`).
- `vc_ab` runs agent-browser in `$VC_BROWSER_SESSION`. Sessions are host-global, so never use another name. Prefer `find label`, `find role ... --name`, `data-testid` and page titles (`<Page> - Coder`) over refs from an old snapshot.
- `site/e2e/helpers.ts` has a Playwright recipe for most pages. Translate it, then check the labels against the page source, because the e2e matchers ignore case.
- CLI: `vc_coder <args>` runs `scripts/coder-dev.sh` with `CODER_DEV_URL=$VC_API_URL`. The shim uses `.coderv2` and drops `CODER_URL` and `CODER_SESSION_TOKEN`, so it never talks to the deployment your workspace runs on or to a URL an older run saved in `.coderv2`.
- API: `vc_api /api/v2/<path>` GETs as the admin and fails on an HTTP error. `vc_api_status /api/v2/<path>` prints only the status code. Both keep the token out of the process list. Never echo `.coderv2/session` or pass it as an argument.
- Logs: `$VC_LOG` holds the `api`, `site` and `build` streams. Launch with `scripts/launch.sh -- --verbose` for debug lines.

## Hand-driven checks

- Put each sub-feature's files in one `$VC_EVIDENCE/<UTC stamp>-<sub-feature id>/` folder, for example `20261105T090000Z-users-create-cli/`. Name files `<NN>-<entry point>-<state>.<ext>`, such as `01-cli-create.txt` or `02-ui-users-after.png`.
- Record each check with `vc_check <name> <command> [args]` and `VC_RESULT_FILE=<folder>/result.txt`. End with `echo "<sub-feature id>: $VC_PASS/$VC_TOTAL checks passed" >>"$VC_RESULT_FILE"`.
- `vc_check` runs its arguments as one command with errexit and pipefail, not as a shell string. Put pipes and redirects inside a function and pass the function. It always returns 0 and prints PASS or FAIL.
- Its counters start at 0 each time `lib.sh` is sourced, so run one sub-feature in one shell.
- Before you create a workspace by hand, run `vc_record_workspace <owner>/<name>`, so cleanup deletes it.

## Cleanup

- `cleanup.sh` deletes only the workspaces recorded in `build/verify-coder/run/workspaces` and lists the rest as kept, because a reused `.coderv2` can hold a developer's own workspaces. When it is about to remove a `.coderv2` that `launch.sh` created, it deletes every workspace in it, because the data goes anyway.
- It closes only `$VC_BROWSER_SESSION`. It stops helpers recorded as `build/verify-coder/run/*.pid` while they still run this checkout's coder binary.
- It sends SIGINT to the develop session it started, then TERM and KILL to leftovers of that session only, and checks that both ports are free. It never kills by process name.
- It removes `.coderv2` only when `launch.sh` created it (marker `.coderv2/.verify-coder-created`) and no other process has its Postgres path on the command line. The marker goes last, so a failed removal can be retried. `--keep-data` deletes the marker, so later cleanups keep the folder too.
- It moves `develop.log` and `build.log` into `build/verify-coder/evidence/` and never deletes that folder. Each step's full output goes to `build/verify-coder/evidence/<stamp>-cleanup.log`.
- With the server already down, it cannot delete workspaces. It then fails and names every `coder-*` container and volume created since the launch, or fails when Docker cannot list them.
- A FAIL line keeps the run state, so you can fix the cause and run it again.

## Gotchas

- Inside a Coder workspace (`CODER_AGENT_URL` set), the dev build names the session cookie `dev_coder_session_token` instead of `coder_session_token` (`scripts/build_go.sh`). Sign in through the form instead of setting cookies by hand.
- After `agent-browser cookies clear`, a cached `/login` breaks CSRF in the dev site and every POST answers 400. `vc_browser_login` loads `/login` with a unique query to avoid it. See `features/sign-in.md`.
- At a load average near 30 on 8 cores, pages sometimes took more than 25 seconds to render, which is agent-browser's default timeout. `vc_ab` raises it to 90 seconds (`VERIFY_CODER_BROWSER_TIMEOUT_MS`). A failed journey step saves `fail-<step>.png` and its page errors, console and network requests next to `steps.log`.
- `vc_browser_login` fills the password through `agent-browser batch` on stdin. Other `vc_ab fill` calls put their text in the process list, so use them only for throwaway values.
