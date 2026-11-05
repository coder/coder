# Coder verification map

This folder is the maintained source for verifying the Coder features listed below. Features not listed have no recipe yet. Read the index before you drive the app, then use the matching feature file as the recipe. Commands below are relative to `.agents/skills/verify-coder/`.

## Baseline preconditions

- `scripts/launch.sh` started the instance from this checkout and `scripts/doctor.sh` prints `doctor: 11/11 checks passed`.
- `source scripts/lib.sh && vc_load_run` exported `VC_WEB_URL`, `VC_API_URL` and `VC_BROWSER_SESSION`.
- Seed data from `scripts/develop/main.go`: owner `admin@coder.com`, user `member@coder.com`, both with the password in `$VC_ADMIN_PASSWORD` (random when `launch.sh` created the database, see SKILL.md, Launch), the default organization `coder`, and the `docker` template when Docker works on the host.
- Nothing seeds an AI provider, a model, a license, a workspace or a second organization.

## Driving conventions

- Browser: `vc_ab <agent-browser args>` and `vc_browser_login [email password]`. Prefer `find label`, `find role <role> --name`, `data-testid` and page titles of the form `<Page> - Coder`.
- CLI: `scripts/coder.sh <args>`. The first line of output is a dev-version warning on stderr.
- API reads: `vc_api /api/v2/<path>`.
- Never drive an instance that `scripts/doctor.sh` does not pass.

## Proof and skip reporting

- Capture the action and the resulting state: a screenshot before and after, and an accessibility snapshot of the result.
- Prove each mutation from a second view: the API or the CLI after a UI change, the UI after a CLI change.
- Put hand-driven artifacts in `$VC_EVIDENCE/<UTC stamp>-<sub-feature id>/`, named `<NN>-<entry point>-<state>.<ext>` (see SKILL.md, Evidence).
- Report an entry point you could not reach with the command you ran and the missing precondition. Do not report it as verified through another entry point.

## Feature entry contract

Each feature file has an H1 title and one paragraph on the user-visible behavior, then these H2 sections in order: `Sub-features`, `How to get to it (user POV)`, `Driving it with agent-browser and the coder CLI`, `Gotchas`.

## Features

- [Sign in](./sign-in.md) covers the password form, the redirect to workspaces, and CLI login. Journey: `vc_browser_login` in `scripts/lib.sh`.
- [Users](./users.md) covers creating a password user in the UI and the CLI, assigning a role, and proving it through the API and a fresh sign-in. Journey: `journeys/create-user.sh`.
- [Templates](./templates.md) covers the template list, the seeded `docker` template, creating from the starter gallery, `coder templates pull` and `push`, and delete. Journey: `journeys/templates.sh`.
- [Workspaces](./workspaces.md) covers creating a Docker workspace in the UI and the CLI, agent connection, `coder ssh`, the web terminal, stop, start and delete. Journey: `journeys/workspace.sh`.
- [Coder Agents](./coder-agents.md) covers provider and model setup, then a chat with a reply from a local fake LLM (`llm-mock`). Journey: `journeys/coder-agents.sh`.
