# Codex webhook controller (experimental prototype)

> [!WARNING]
> This is a dogfood-only prototype. It is not part of the Coder build, it is
> not supported, and its interfaces can change or disappear at any time. Do
> not run it against production deployments or reuse it as a reference for
> production code.

Prototype generated with Coder Agents on behalf of david-fraley.

## What it does

The controller connects one OpenAI Agents session to one Coder workspace
that it creates on demand:

1. An OpenAI Agents session that uses a `self_hosted` environment asks for
   an environment connection, and OpenAI sends a signed webhook.
1. The controller receives the webhook on `POST /webhooks/openai`, checks the
   signature, and queues the event in a local SQLite database.
1. The worker loop fetches the session from the OpenAI API and checks it
   against the configured agent, the `/home/coder/demo` directory, and an
   `https://api.openai.com/v1/agents/api/connect/...` executor URL.
1. In `allocate` mode with `provision: "create"`, the controller creates the
   worker workspace through the Coder REST API from the fixed `coder`
   template, waits until it is running, and prepares it over `coder ssh`.
   Preparation installs the pinned Codex CLI, writes a demo marker file, and
   builds the executor image from [`Dockerfile.executor`](./Dockerfile.executor)
   with the workspace's Docker daemon.
1. The controller starts one locked-down container in the workspace. Its
   entrypoint, [`executor-entrypoint.py`](./executor-entrypoint.py), runs
   `codex exec-server --remote ...`, which opens an outbound connection to
   OpenAI and runs the session's commands in `/home/coder/demo`.

The receiver accepts no Coder or administrative calls. Session creation,
setup, and cleanup are separate local commands.

## Scope and restrictions

These limits are intentional. Do not generalize them without a design
review.

- **One session, one workspace.** Each state directory holds one configured
  agent, one session allocation, and one worker workspace. A second session
  is rejected with a capacity error. There is no pool, scheduler, UI, or
  multi-user support.
- **Fixed organization and template.** Provisioning is allowlisted to the
  `coder` organization and the template with ID
  `0d286645-29aa-4eaf-9b52-cc5d2740c90b` (see
  [`src/provision.ts`](./src/provision.ts)). The create request sets
  `Select IDEs=[]`, a 2 hour autostop, and the template's default preset.
- **Owner only.** The authenticated Coder user must be the owner named in
  `WORKSPACE` and a member of the `coder` organization. The controller never
  creates workspaces for other users.
- **No adoption.** If the target workspace name already exists and this
  state directory has no claim for it, the controller refuses to use it. Pick
  a new workspace name for each run.
- **Fixed paths and endpoints.** The worker directory is `/home/coder/demo`,
  the only OpenAI origin is `https://api.openai.com`, and the Codex binary is
  the `x86_64-unknown-linux-musl` build.
- **Manual diagnostics.** `src/live-adapter-check.ts` and
  `src/local-stop-check.ts` require `WORKSPACE=owner/name` pointing to a
  dedicated, already prepared workspace. They do not create it and retain
  assumptions about the original test fixtures and pinned local image.
  The live adapter check writes evidence under this package's `state/`,
  independently of `STATE_DIR`. Use the webhook flow below for on-demand runs.

## Requirements

- Node.js 22.19 or later (the store uses the built-in `node:sqlite` module).
- pnpm 10.33.2.
- The `coder` CLI on `PATH`, authenticated to the dogfood deployment.
- Python 3 on `PATH`. The unit tests run the embedded workspace helpers with
  it.
- `CODER_URL` and `CODER_SESSION_TOKEN` set in the controller's environment
  for the user who will own the worker workspace.
- An OpenAI application API key and a separate executor API key with access
  to the Agents beta.

## Install and test

```sh
cd experimental/codex-controller
pnpm install --frozen-lockfile
pnpm test
pnpm typecheck
```

## Credentials

The scripts read credentials from `~/.config/codex-spike/credentials.json`.
Create the file with owner-only permissions and enter values at hidden
prompts, so they never appear in shell history or arguments:

```sh
python3 - <<'EOF'
import getpass, json, os, pathlib
path = pathlib.Path.home() / '.config/codex-spike/credentials.json'
path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
data = {
    'OPENAI_API_KEY': getpass.getpass('Application API key: '),
    'OPENAI_EXECUTOR_API_KEY': getpass.getpass('Executor API key: '),
}
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as f:
    json.dump(data, f)
    f.write('\n')
EOF
```

`OPENAI_API_KEY` is the application key that the controller, `submit`, and
`end` use. `OPENAI_EXECUTOR_API_KEY` is passed only to the executor
container over stdin. The `register` step adds `OPENAI_WEBHOOK_SECRET` to
the same file.

Set the Coder session token the same way:

```sh
export CODER_URL=<your dogfood deployment URL>
read -rs CODER_SESSION_TOKEN && export CODER_SESSION_TOKEN
```

## Run one session

Use a new, isolated state directory for every run. It holds the agent
configuration, the event queue, the provisioning claim, the webhook endpoint
record, and the trace logs:

```sh
export STATE_DIR="$PWD/state/run-$(date +%Y%m%d-%H%M%S)"
```

1. **Create the agent.** `WORKSPACE` names the worker workspace to create.
   It must be `<your-username>/<new-name>` and must not exist yet:

   ```sh
   WORKSPACE=<your-username>/<new-name> pnpm exec tsx src/setup.ts agent
   ```

   This writes `$STATE_DIR/config.json` with on-demand creation defaults.

1. **Choose the mode.** Edit `$STATE_DIR/config.json`. Set
   `"mode": "allocate"` and `"provision": "create"` to run commands. An
   optional first pass with `"mode": "audit"` verifies and logs connection
   requests without creating a workspace or starting an executor.

1. **Expose only the webhook route.** The receiver listens on
   `127.0.0.1:8791` and answers only `POST /webhooks/openai` with a valid
   signature. Share port 8791 of the workspace that runs the controller at
   the level your deployment's
   [port sharing policy](../../docs/admin/networking/port-forwarding.md#configure-maximum-port-sharing-level)
   already allows. See [Share ports](../../docs/user-guides/workspace-access/port-forwarding.md#share-ports).
   Do not change deployment policy for this prototype.

1. **Register the webhook endpoint.** Registration requires an explicit
   approval variable and an HTTPS URL:

   ```sh
   APPROVE_PUBLIC_WEBHOOK=yes \
   WEBHOOK_URL=https://<shared-port-url>/webhooks/openai \
   pnpm exec tsx src/setup.ts register
   ```

1. **Start the receiver** in its own terminal with the same `STATE_DIR`,
   `CODER_URL`, and `CODER_SESSION_TOKEN`:

   ```sh
   pnpm start
   ```

   Wait for the `receiver_ready` log line.

1. **Submit a task** from another terminal with the same `STATE_DIR`:

   ```sh
   pnpm submit
   ```

   The script creates a session, sends one task, streams events until the
   turn ends, and prints the session ID (`sess_...`). It leaves the session
   in place for inspection.

1. **Inspect the real output.** Read the evidence that `submit` saved and
   compare it with the files in the worker workspace:

   - `$STATE_DIR/<session-id>/events.jsonl` and `items.json`: the session's
     events and the agent's reported results.
   - `$STATE_DIR/trace.jsonl`: the controller's redacted trace, including
     `signed_delivery`, `provision_*`, and `assigned_executor`.
   - The worker files:

     ```sh
     coder ssh --disable-autostart "$WORKSPACE" -- \
       cat /home/coder/demo/worker-marker.txt /home/coder/demo/webhook-proof.txt
     ```

     The marker is random for each workspace, so a match shows that the
     command ran in that workspace.

## Clean up

Run these steps in order, with the same `STATE_DIR`:

1. Stop the receiver with Ctrl+C, so it cannot race the cleanup.
1. End the session. This stops the executor container, deletes the OpenAI
   session, and stops (does not delete) the worker workspace:

   ```sh
   pnpm exec tsx src/end.ts <session-id>
   ```

1. Delete the webhook endpoint:

   ```sh
   pnpm exec tsx src/setup.ts unregister
   ```

1. Remove the port 8791 share by hand from the controller workspace.
1. Delete the stopped worker workspace with `coder delete` when you no
   longer need its files.

## Keep secrets and runtime files out of Git

Never commit credentials, state directories, SQLite files, traces, or
session evidence. The [`.gitignore`](./.gitignore) excludes `state/`,
`node_modules/`, `credentials.json`, and SQLite and log files. The
credentials file lives outside the repository. Run `git status` before
committing to confirm that only source files changed.

## Results so far

Earlier end-to-end runs completed with real webhook delivery, on-demand
workspace creation, and commands that ran in the worker workspace. In those
runs, preparing a cold workspace took longer than OpenAI's environment
connection wait allowed. The first task timed out and succeeded only after
an explicit retry of the task. This prototype does not show that the first
input on a cold workspace is handled seamlessly.

## Compatibility

Only these pinned versions have been tested with the prototype:

- Codex CLI `0.156.0-alpha.2` (`CODEX_VERSION` in
  [`src/provision.ts`](./src/provision.ts)).
- OpenAI Node SDK `openai@7.23.0`. The SDK's webhook endpoint types omit the
  Agents event names, so `setup.ts register` sends them in a raw request.

Agents beta APIs, event names, and the `exec-server` flags can change
without notice. Other versions may fail.

The executor image is built inside the worker workspace's Docker daemon and
referenced by its immutable `sha256:` ID. In `provision: "create"` mode,
each preparation rebuilds the image and uses the new ID. Outside that mode,
the controller uses the fixed `DOCKER_IMAGE` constant in
[`src/docker.ts`](./src/docker.ts). After you rebuild the image, or if the
workspace loses its Docker image store, rebuild it and update that constant.
