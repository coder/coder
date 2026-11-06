---
name: verify-coder
description: "Launch an isolated local Coder from this checkout with scripts/develop.sh, check it with a doctor, drive it the way a user would through the web UI (agent-browser) and the coder CLI, and capture evidence that survives cleanup. Use to prove a Coder change works end to end, to reproduce a bug on the real app, or before calling UI, API or CLI work done. For a PR-scoped dogfood report, use the dogfood skill for scoping and this skill for the run."
---

# Verify Coder

Drive the real app, not tests. Every script prints one line per check and ends with a count line such as `doctor: 11/11 checks passed`. A run without its count line is unchecked.

Script paths are relative to this skill's directory, `.agents/skills/verify-coder/`, and the scripts work from any working directory. `build/verify-coder/` and `.coderv2/` are relative to the repo root. `reference.md` has the details behind each step.

## 1. Launch

```sh
scripts/launch.sh               # reuse this checkout's .coderv2 data
scripts/launch.sh --fresh       # destroy this checkout's dev database first
scripts/launch.sh -- --verbose  # after --, only the server flags launch.sh allows
```

- Run it in the foreground. It returns once the instance is ready, and the instance keeps running in its own session.
- It refuses data from a plain `develop.sh` run, whose admin has the published default password. Use `--fresh` for that data.

## 2. Doctor

```sh
scripts/doctor.sh
```

Run it first whenever anything looks off. A FAIL on `build` means a stale binary: clean up and relaunch.

## 3. Drive

Load the instance in every new shell, each separate tool call included:

```sh
source scripts/lib.sh && vc_load_run
```

- Web UI: `vc_browser_login`, then `vc_ab <agent-browser args>` against `$VC_WEB_URL`.
- CLI: `vc_coder <coder args>`.
- API: `vc_api /api/v2/<path>` and `vc_api_status /api/v2/<path>`.
- Journeys, each one feature end to end with its own evidence:
  - `journeys/create-user.sh` (9 checks, about 15 seconds)
  - `journeys/templates.sh` (4 checks, needs Docker)
  - `journeys/workspace.sh` (10 checks, about 2 minutes, needs Docker)
  - `journeys/coder-agents.sh` (7 checks, starts its own fake LLM)
- `features/README.md` maps each feature to its entry points and proofs. A proof through one entry point is incomplete when the feature file lists others.

## 4. Evidence

- Journeys write `build/verify-coder/evidence/<UTC stamp>-<journey>/` with screenshots, read-backs, `result.txt` and `steps.log`. For hand-driven checks, follow "Hand-driven checks" in `reference.md`.
- Prove the real user path, the UI form or the CLI command, and check the side effect through a second view such as the API.
- Never screenshot a form after typing a real secret, and never save output that echoes one.
- Mock only where a production boundary already isolates the external system, such as an LLM provider.

## 5. Cleanup

```sh
scripts/cleanup.sh              # also removes .coderv2 when launch.sh created it
scripts/cleanup.sh --keep-data  # keep .coderv2, and treat it as yours from then on
```

Run it after every attempt, failed ones too. A FAIL line keeps the run state, so fix the cause and run it again. It deletes only the workspaces the harness recorded. Run `vc_record_workspace <owner>/<name>` before you create one by hand.
