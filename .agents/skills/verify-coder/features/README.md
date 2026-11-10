# Coder verification map

This folder is the maintained source for verifying the Coder features listed below. Features not listed have no recipe yet. Read the index before you drive the app, then use the matching feature file as the recipe. Commands below are relative to `.agents/skills/verify-coder/`.

## Baseline preconditions

- `scripts/launch.sh` started the instance from this checkout and `scripts/doctor.sh` prints `doctor: 12/12 checks passed`.
- `source scripts/lib.sh && vc_load_run` exported `VC_WEB_URL` and `VC_API_URL`.
- Seed data from `scripts/develop/main.go`: owner `admin@coder.com`, user `member@coder.com`, both with the password in `$VC_ADMIN_PASSWORD` (random when `launch.sh` created the database, see "Admin password" in `reference.md`), the default organization `coder`, and the `docker` template when Docker works on the host.
- Nothing seeds an AI provider, a model, a license, a workspace or a second organization.

## Driving conventions

- Browser: none. This skill drives no browser, so each feature file names its web UI sub-features as not verified.
- CLI: `vc_coder <args>`. The first line of output is a dev-version warning on stderr.
- API reads: `vc_api /api/v2/<path>`.
- Never drive an instance that `scripts/doctor.sh` does not pass.

## Proof and skip reporting

- Capture the action and the resulting state: the command output, and an API read before and after.
- Prove each mutation from a second view: the API after a CLI change.
- Put hand-driven artifacts in `$VC_EVIDENCE/<UTC stamp>-<sub-feature id>/`, named `<NN>-<entry point>-<state>.<ext>` (see "Hand-driven checks" in `reference.md`).
- Report an entry point you could not reach with the command you ran and the missing precondition. Do not report it as verified through another entry point.

## Feature entry contract

Each feature file has an H1 title and one paragraph on the user-visible behavior, then these H2 sections in order: `Sub-features`, `How to get to it (user POV)`, `Driving it with the coder CLI and the API`, `Gotchas`.

## Features

- [Sign in](./sign-in.md) covers the password form, the redirect to workspaces, and CLI login. The CLI drives `signin-cli` only.
- [Users](./users.md) covers creating a password user in the UI and the CLI, assigning a role, and a fresh sign-in. The CLI and the API drive `users-create-cli` only.
- [Templates](./templates.md) covers the template list, the seeded `docker` template, creating from the starter gallery, `coder templates pull` and `push`, and delete. The CLI and the API drive every sub-feature except `templates-page`, and create from a starter with `coder templates init` instead of the gallery.
- [Workspaces](./workspaces.md) covers creating a Docker workspace in the UI and the CLI, agent connection, `coder ssh`, the web terminal, stop, start and delete. The CLI and the API drive every sub-feature except `ws-create-ui`, `ws-terminal` and the UI half of `ws-lifecycle`.
