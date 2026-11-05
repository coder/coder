---
name: verify-coder
description: "Launch an isolated local Coder from this checkout with scripts/develop.sh, check it with a read-only doctor, drive it the way a user would through the web UI (agent-browser) and the coder CLI, and capture evidence that survives cleanup. Use to prove a Coder change works end to end, to reproduce a bug on the real app, or before calling UI, API or CLI work done. For a PR-scoped dogfood report, use the dogfood skill for scoping and this skill for the run."
---

# Verify Coder

Drive the real app, not tests. Every helper prints one line per check and ends with a count line such as `doctor: 7/7 checks passed`. A run without its count line is unchecked.

Paths below are relative to this skill's directory (`.agents/skills/verify-coder/`). Run the scripts from anywhere inside the checkout.

## Launch

```sh
scripts/launch.sh            # reuse this checkout's .coderv2 data
scripts/launch.sh --fresh    # destroy this checkout's dev database first (--db-reset)
scripts/launch.sh -- --use-proxy   # extra flags go to scripts/develop.sh
```

- It builds `build/.bin/develop` and `build/coder_<os>_<arch>` first, then picks free API and web ports (13000 and 18000 plus a per-checkout offset), then starts `scripts/develop.sh` in its own process session. Coder metrics are off (`--prometheus-port 0`).
- The first build in a checkout compiles the site and seven slim binaries. It took 13 minutes on 8 cores at load average 15 to 27. With the build done, startup took about 40 seconds on a new database and 9 seconds on kept data.
- Ready means the develop banner `Coder is now running in development mode` appeared, which comes after migrations, the first user, the `member` user and the `docker` starter template. Then `launch.sh` runs the doctor and exits with its result.
- Seed data: `admin@coder.com` (owner) and `member@coder.com`, both with password `SomeSecurePassword!` (`CODER_DEV_ADMIN_PASSWORD` overrides it). The `docker` template exists only when `docker info` works.
- It refuses to start when `build/verify-coder/run/env` records an instance. It never reuses a Coder it did not start: `develop.sh` itself would silently reuse any Coder on its API port, so never point `CODER_DEV_PORT` at a busy port.
- It unsets `ANTHROPIC_*` and `OPENAI_*` keys for the server, so nothing spends real tokens unless you configure a provider in the UI.
- State: `build/verify-coder/run/` (env file, `build.log`, `develop.log`) and `.coderv2/` (built-in Postgres, CLI session). Both are per checkout and gitignored.

## Doctor

```sh
scripts/doctor.sh
```

Read-only. Run it first whenever anything looks off. It checks that the recorded pid is this checkout's `build/.bin/develop` session leader, that both ports belong to that session, `/healthz`, that the `/api/v2/buildinfo` version (`-devel+<short sha>`) is `HEAD` (FAIL means stale binary: clean up and relaunch), that the CLI session in `.coderv2/session` is the admin, and that the web port proxies `/api`. It prints the URLs and template names as INFO lines.

## Drive

Load the instance first: `source scripts/lib.sh && vc_load_run` gives `VC_WEB_URL`, `VC_API_URL`, `VC_BROWSER_SESSION` and the helpers below.

- Web UI: drive `$VC_WEB_URL` (the Vite dev server, which proxies `/api` to the API port) with `agent-browser --session "$VC_BROWSER_SESSION"` (`vc_ab` in `lib.sh`). Never use another session name: sessions are host-global. `vc_browser_login [email password]` signs in through the real form. Prefer `find label`, `find role ... --name`, `data-testid` and page titles (`<Page> - Coder`) over refs from an old snapshot. `site/e2e/helpers.ts` has a working Playwright recipe for most pages: translate it, then check the labels against the page source because the e2e matchers ignore case.
- CLI: `scripts/coder.sh <args>` runs the binary develop built with `--global-config .coderv2`, logged in as the admin. It unsets `CODER_URL` and `CODER_SESSION_TOKEN` so it never talks to the deployment this workspace runs on.
- API: `vc_api /api/v2/<path>` GETs as the admin without printing the token. Never echo `.coderv2/session`.
- Logs: `$VC_LOG` holds the `api`, `site` and `build` streams. Add `-- --trace` to the launch for more.
- Journeys: `journeys/<name>.sh` scripts drive one mapped feature end to end and write evidence. Run one to prove the harness, or copy one for a new feature.

## Evidence

- Each journey writes `build/verify-coder/evidence/<UTC stamp>-<journey>/`: numbered screenshots of the action and the result, an accessibility snapshot of the final page, the API and CLI read-backs as JSON, `result.txt` with the check lines and count line, and `steps.log` with each step's full output.
- Prove the real user path: the UI form or the CLI command a user runs, not a test endpoint or a direct DB write. Capture the state before and after the action, not only the final screen.
- Verify side effects next to what is visible: rows through the API, CLI output, files, containers (`docker ps`), log lines.
- Pass absolute paths to `agent-browser screenshot`. Relative paths resolve against the daemon's working directory.
- Never screenshot or snapshot a form after typing a real secret. Journeys use throwaway passwords and screenshot before typing them.
- Mocks only where a production boundary already isolates the external system, for example an OpenAI-compatible fake for an LLM provider.

## Cleanup

```sh
scripts/cleanup.sh             # also removes .coderv2 when launch.sh created it
scripts/cleanup.sh --keep-data # keep .coderv2 for the next launch
```

It deletes the instance's workspaces through the CLI, closes only `$VC_BROWSER_SESSION`, sends SIGINT to the develop session it started and then TERM and KILL to leftovers of that session only, checks both ports are free, and moves `develop.log` and `build.log` into `build/verify-coder/evidence/`. It never kills by process name and never deletes `build/verify-coder/evidence/`. Run it after every failed attempt too. A FAIL line keeps the run state so you can fix the cause and run it again.

## Features

`features/README.md` indexes the user-facing features with how to reach, drive and prove each one. A proof that drives one convenient entry point is incomplete when the feature file lists others. When a journey fails because seed data lacks a precondition, report the failure and record the reason in that feature's Gotchas. Never weaken the journey to make it pass.

## Gotchas

- Inside a Coder workspace (`CODER_AGENT_URL` set) the dev build names the session cookie `dev_coder_session_token` instead of `coder_session_token` (`scripts/build_go.sh`). Sign in through the form instead of setting cookies by hand.
- After `agent-browser cookies clear`, a cached `/login` breaks CSRF in the dev site and every POST answers 400. Sign in with `vc_browser_login`, which loads `/login` with a unique query. See `features/sign-in.md`.
- `launch.sh` sets `umask 022` for the build and the server. Some agent shells default to 077.
- A second checkout can run its own instance at the same time. Two instances in one checkout cannot: they share `.coderv2`.
