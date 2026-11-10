# Users

An owner adds a user who signs in with a password, from the admin users page or the CLI. The new user appears in the users list and can sign in.

## Sub-features

- `users-create-ui` creates a password user on `/deployment/users/create`.
- `users-create-cli` creates a password user with `coder users create`.
- `users-roles` assigns roles from the row menu.
- `users-login` proves the new user can sign in.

## How to get to it (user POV)

- `Admin settings` > `Users`, or `$VC_WEB_URL/deployment/users`, then the `New user` link.
- Run `coder users create` in a terminal.

## Driving it with the coder CLI and the API

Preconditions:

- `scripts/doctor.sh` passes.

- **Web UI.** This skill drives no browser, so report `users-create-ui` and `users-roles` as not verified.
- **Sign in as the user.** The CLI discards the generated password (see below), so report `users-login` as not verified.
- **CLI create.** `vc_coder users create --username <u> --email <u>@coder.com --full-name "<name>" >/dev/null 2>&1` and check its exit code. Without `--password`, the CLI generates a password and prints it on stderr, so discard both streams. `vc_api "/api/v2/users?q=<u>"` returns one user with `login_type` `password` and status `dormant`, and `vc_coder users list --output json` includes the username.

## Gotchas

- `coder users create` prints `Your password is: <password>` on stderr. Never pass `--password`, which puts the password in the process list, and never save its output as evidence.
- New users show `Dormant` until their first sign-in. That is not a failure.
