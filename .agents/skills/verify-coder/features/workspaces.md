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

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes, and `scripts/coder.sh templates list --output json | jq -r '.[].Template.name'` prints `docker`.
- `docker info` works for the user that runs the instance.

- **Whole journey.** Run `journeys/workspace.sh`. It covers every sub-feature below, ends with `workspace: 9/9 checks passed` in about 90 seconds with a cached image, and writes evidence to `build/verify-coder/evidence/<stamp>-workspace/`.
- **Create in the CLI.** `scripts/coder.sh create <name> --template docker --parameter 'jetbrains_ides=[]' --yes </dev/null`. It streams the build and prints `The <name> workspace has been created`.
- **Build state.** `vc_api /api/v2/users/admin/workspace/<name>` has `latest_build.transition` and `latest_build.status`. A finished start reads `start`/`running`, a finished stop `stop`/`stopped`, a failure `.../failed`.
- **Agent connected.** Poll until `latest_build.resources[].agents[]` has `status` `connected` and `lifecycle_state` `ready` (agent name `main`). `docker ps` then lists `coder-admin-<name>` from image `codercom/example-base:ubuntu`.
- **SSH.** `scripts/coder.sh ssh <name> -- echo verify-ok 2>/dev/null` prints `verify-ok`. Stderr carries the dev-version warning.
- **Create in the UI.** `vc_ab open "$VC_WEB_URL/templates/docker/workspace"`, wait for the title `Create workspace - Coder`, then `vc_ab wait 'form[aria-label="Create workspace form"]'` and `vc_ab wait --load networkidle`. The name field renders after the template loads. Then `vc_ab find label "Workspace name" fill <name>` and `vc_ab find role button click --name "Create workspace"`. The URL becomes `/@admin/<name>`.
- **Agent in the UI.** `vc_ab wait '[data-testid=agent-status-ready]'`. An element's text reads `Workspace status: Running`.
- **Web terminal.** `vc_ab find role link click --name "Terminal"` opens a new tab at `/@admin/<name>.main/terminal?reconnect=<id>`. `vc_ab wait '.xterm'`, `vc_ab click '.xterm'`, `vc_ab keyboard type "echo ok > /tmp/verify-web-terminal"`, `vc_ab press Enter`. Prove it from a second view: `scripts/coder.sh ssh <name> -- cat /tmp/verify-web-terminal` prints `ok`. Return to the workspace tab with `vc_ab tab list` and `vc_ab tab <id>`.
- **Stop and start in the UI.** Wait for `[data-testid=workspace-stop-button]`, click it, and poll the API for `stop`/`stopped`. The page reads `Workspace status: Stopped` and `docker ps` has no container. Then click `[data-testid=workspace-start]` and poll for `start`/`running`.
- **Stop and start in the CLI.** `scripts/coder.sh stop <name> --yes` and `scripts/coder.sh start <name> --yes`, with the same API reads.
- **Delete in the UI.** Click `[data-testid=workspace-options-button]`, then `vc_ab find role menuitem click --name "Delete…"`, `vc_ab fill '[role=dialog] input' <name>`, and `vc_ab find role button click --name "Delete" --exact`. `vc_api_status /api/v2/users/admin/workspace/<name>` prints `404`, `docker ps -a` has no `coder-admin-<name>`, and `docker volume inspect coder-<workspace id>-home` fails.
- **Delete in the CLI.** Read the ID first with `vc_api /api/v2/users/admin/workspace/<name> | jq -r .id`. Then `scripts/coder.sh delete <name> --yes`. `vc_api_status /api/v2/users/admin/workspace/<name>` prints `404`, `docker ps -a` has no `coder-admin-<name>`, and `docker volume inspect coder-<id>-home` fails. `scripts/cleanup.sh` deletes every workspace that is left.

## Gotchas

- Every build runs `terraform init`, which downloads the `coder/coder` and `kreuzwerker/docker` providers from GitHub releases. A GitHub 504 fails the build with `Failed to install provider`. Read the cause with `grep level=ERROR "$VC_LOG"` and report it as an environment failure.
- The `docker` template has one parameter, `jetbrains_ides` (`list(string)`, a multi-select). Without `--parameter`, `coder create` prompts for it, and with no TTY the prompt sends a value that fails `terraform plan` with `When using list(string) type, value must be a json encoded list of strings`.
- The template rewrites `127.0.0.1` in the agent URL to `host.docker.internal`, mapped to the host gateway. The API binds `0.0.0.0`, so agents reach it from a local Docker daemon (`unix:///var/run/docker.sock`). A remote daemon was not tested.
- The first workspace pulls `codercom/example-base:ubuntu`. Allow several minutes.
- The agent name in the `docker` template is `main`.
- About 6 seconds into a build, the workspace page scrolls down 72 px to follow the build log. The header buttons (`Stop`, `Start`, the options button) then sit under the sticky navbar, and a click lands on the navbar. Run `vc_ab eval 'window.scrollTo(0, 0)'` before clicking them.
- Workspace containers and home volumes outlive a stopped server. Delete workspaces before `scripts/cleanup.sh` stops the server, or let it do so.
