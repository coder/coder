# Workspaces

A user creates a workspace from a template, waits for its agent to connect, opens a shell over SSH or the web terminal, and stops, starts and deletes it.

## Sub-features

- `ws-create-ui` creates a workspace on `/templates/docker/workspace`.
- `ws-create-cli` creates a workspace with `coder create`.
- `ws-agent` proves the agent connected.
- `ws-ssh` runs a command with `coder ssh`.
- `ws-terminal` opens the web terminal.
- `ws-lifecycle` stops, starts and deletes the workspace.

## How to get to it (user POV)

- The `Create Workspace` link on `/templates/docker`, or `$VC_WEB_URL/templates/docker/workspace`.
- The workspace page `$VC_WEB_URL/@<user>/<workspace>`.
- Run `coder create`, `coder ssh`, `coder stop`, `coder start` or `coder delete` in a terminal.

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes, and `scripts/coder.sh templates list --output json | jq -r '.[].Template.name'` prints `docker`.
- `docker info` works for the user that runs the instance.

- **Create in the UI.** `vc_ab open "$VC_WEB_URL/templates/docker/workspace"`, fill `find label "Workspace name"`, then `vc_ab find role button click --name "Create workspace"`. The URL becomes `/@admin/<name>`.
- **Agent connected.** Poll the API until the agent connects: `until vc_api /api/v2/users/admin/workspace/<name> | jq -e '[.latest_build.resources[].agents[]?.status] | index("connected")' >/dev/null; do sleep 5; done`. Then `vc_ab wait '[data-testid=agent-status-ready]'` on the workspace page, which reads `Workspace status: Running`. Second view: `vc_api /api/v2/users/admin/workspace/<name>` has `latest_build.status` `running` and `latest_build.resources[].agents[].status` `connected`. `docker ps` lists `coder-admin-<name>`.
- **Create in the CLI.** `scripts/coder.sh create <name> --template docker --parameter 'jetbrains_ides=[]' --yes </dev/null`. It streams the build and exits 0 when the workspace runs.
- **SSH.** `scripts/coder.sh ssh <name> -- echo verify-ok` prints `verify-ok`.
- **Web terminal.** On the workspace page click the `Terminal` link. It opens `/@admin/<name>.main/terminal`. Type a command and screenshot the output.
- **Stop and start.** `scripts/coder.sh stop <name> --yes`, then `scripts/coder.sh start <name> --yes`. In the UI the `Stop` button is `[data-testid=workspace-stop-button]` and `Start` is `[data-testid=workspace-start]`.
- **Delete.** `scripts/coder.sh delete <name> --yes`, or in the UI `[data-testid=workspace-options-button]`, `Delete…`, type the name into `Workspace name`, then `Delete`. `docker ps -a` no longer lists the container. `scripts/cleanup.sh` deletes every workspace that is left.

## Gotchas

- Every build runs `terraform init`, which downloads the `coder/coder` and `kreuzwerker/docker` providers from GitHub releases. A GitHub 504 fails the build with `Failed to install provider`. Read the cause with `grep level=ERROR "$VC_LOG"` and report it as an environment failure.
- The `docker` template has one parameter, `jetbrains_ides` (`list(string)`, a multi-select). Without `--parameter`, `coder create` prompts for it, and with no TTY the prompt sends a value that fails `terraform plan` with `When using list(string) type, value must be a json encoded list of strings`.
- The template rewrites `127.0.0.1` in the agent URL to `host.docker.internal`, mapped to the host gateway. The API binds `0.0.0.0`, so this works with local Docker. With a remote Docker daemon the agent cannot reach the API and stays `Connecting...`.
- The first workspace pulls `codercom/example-base:ubuntu`. Allow several minutes.
- The agent name in the `docker` template is `main`.
- Workspace containers and home volumes outlive a stopped server. Delete workspaces before `scripts/cleanup.sh` stops the server, or let it do so.
