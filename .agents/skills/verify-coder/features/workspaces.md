# Workspaces

A user creates a workspace from a template, waits for its agent to connect, opens a shell over SSH or the web terminal, and stops, starts and deletes it.

## Sub-features

- `ws-create-ui` creates a workspace on `/templates/docker/workspace`.
- `ws-create-cli` creates a workspace with `coder create`.
- `ws-agent` proves the agent connected.
- `ws-ssh` runs a command with `coder ssh`.
- `ws-terminal` opens the web terminal.
- `ws-lifecycle` stops, starts and deletes the workspace in the UI and the CLI.

## How to get to it (user POV)

- The `Create Workspace` link on `/templates/docker`, or `$VC_WEB_URL/templates/docker/workspace`.
- The workspace page `$VC_WEB_URL/@<user>/<workspace>`.
- Run `coder create`, `coder ssh`, `coder stop`, `coder start` or `coder delete` in a terminal.

## Driving it with the coder CLI and the API

Preconditions:

- `scripts/doctor.sh` passes, and `vc_coder templates list --output json | jq -r '.[].Template.name'` prints `docker`.
- `docker info` works for the user that runs the instance.

- **Create in the CLI.** `vc_coder create <name> --template docker --parameter 'jetbrains_ides=[]' --yes </dev/null`. It streams the build and prints `The <name> workspace has been created`.
- **Build state.** `vc_api /api/v2/users/admin/workspace/<name>` has `latest_build.transition` and `latest_build.status`. A finished start reads `start`/`running`, a finished stop `stop`/`stopped`, a failure `.../failed`.
- **Agent connected.** `vc_wait_agent admin/<name>` polls for about 5 minutes until the agent (`main`) has `status` `connected` and `lifecycle_state` `ready`. It stops at once and fails when the workspace cannot be read or `latest_build.status` reads `failed`. `docker ps` then lists `coder-admin-<name>` from image `codercom/example-base:ubuntu`.
- **SSH.** `vc_coder ssh <name> -- echo verify-ok 2>/dev/null` prints `verify-ok`. Stderr carries the dev-version warning.
- **Web UI.** This skill drives no browser, so report `ws-create-ui`, `ws-terminal` and the UI half of `ws-lifecycle` as not verified.
- **Stop and start in the CLI.** `vc_coder stop <name> --yes`, then poll the API for `stop`/`stopped` and check that `docker ps` has no `coder-admin-<name>`. `vc_coder start <name> --yes`, then poll for `start`/`running` and check that the container runs again.
- **Delete in the CLI.** Read the ID first with `vc_api /api/v2/users/admin/workspace/<name> | jq -r .id`. Then `vc_coder delete <name> --yes`. `vc_api_status /api/v2/users/admin/workspace/<name>` prints `404`, `docker ps -a` has no `coder-admin-<name>`, and `docker volume inspect coder-<id>-home` fails. `scripts/cleanup.sh` deletes only recorded workspaces (see Gotchas), and every workspace when it removes a `.coderv2` that `launch.sh` created.

## Gotchas

- Every build runs `terraform init`, which downloads the `coder/coder` and `kreuzwerker/docker` providers from GitHub releases. A GitHub 504 fails the build with `Failed to install provider`. Read the cause with `grep level=ERROR "$VC_LOG"` and report it as an environment failure.
- The `docker` template has one parameter, `jetbrains_ides` (`list(string)`, a multi-select). Without `--parameter`, `coder create` prompts for it, and with no TTY the prompt sends a value that fails `terraform plan` with `When using list(string) type, value must be a json encoded list of strings`.
- The template rewrites `127.0.0.1` in the agent URL to `host.docker.internal`, mapped to the Docker bridge gateway. The API, Vite and metrics listen on 127.0.0.1 only, so `scripts/launch.sh` runs `scripts/docker-forward.py`, which carries only the API port from the gateway to 127.0.0.1. Agents of a local Docker daemon (`unix:///var/run/docker.sock`) connect through it. A remote daemon was not tested.
- The first workspace pulls `codercom/example-base:ubuntu`. Allow several minutes.
- The agent name in the `docker` template is `main`.
- `scripts/cleanup.sh` deletes only recorded workspaces. Run `vc_record_workspace admin/<name>` before you create one by hand. Record only workspaces of the local Docker daemon, such as ones from the `docker` template.
- Workspace containers and home volumes outlive a stopped server. Delete workspaces before `scripts/cleanup.sh` stops the server, or let it do so.
