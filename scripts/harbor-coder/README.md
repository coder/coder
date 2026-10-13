# harbor-coder

[Harbor](https://github.com/harbor-framework/harbor) integration for Coder:

- `harbor_coder.environment:CoderEnvironment`: a Harbor environment provider
  that runs each trial inside a Coder workspace.
- `harbor_coder.agent:CoderAgentsAgent`: a Harbor agent that drives Coder
  Agents (the coderd chat agent) against that workspace.
- `harbor_coder.bootstrap`: configures a fresh coderd (first user, Anthropic
  provider, chat model, workspace template) for eval runs.
- `harbor_coder.compare`: diffs two Harbor job directories into a Markdown
  verdict.
- `tasks/`: a small Harbor task suite with deterministic verifiers.
- `template/`: the Coder template the environment provider creates
  workspaces from.

The GitHub workflow `eval/coder-agents` (`.github/workflows/eval-coder-agents.yaml`)
runs the suite against coderd built from `main` and from the current branch
and posts the comparison to the step summary. Trigger it manually; trials
spend real model tokens.

## How it fits together

```
harbor run ──► CoderEnvironment.start()
                 docker build task image + coder binary layer
                 coder create <ws> --template harbor-task --parameter image=...
                 wait for agent connected
             ──► CoderAgentsAgent.run()
                 POST /api/v2/chats {workspace_id, content: instruction}
                 poll GET /api/v2/chats/{id} until waiting|error
                 collect messages, tokens, GET .../cost
             ──► verifier: upload /tests, exec test.sh over coder ssh, read reward
             ──► CoderEnvironment.stop(): coder delete <ws>
```

The task container runs `coder agent` directly as its entrypoint (the task
image gets a layer with the `coder` binary), so task base images need no
curl, wget, or shell beyond `bash` and `tar`.

## Local run

```sh
cd scripts/harbor-coder
uv tool install --with-editable . harbor

cd ../..
export ANTHROPIC_API_KEY=...            # or a gateway token
export ANTHROPIC_BASE_URL=...           # optional
./scripts/harbor-coder/serve-coderd.sh .harbor-coder/env 3000

set -a; . .harbor-coder/env; set +a
export PATH="$PWD/.harbor-coder:$PATH"  # the freshly built coder CLI
harbor run -p scripts/harbor-coder/tasks \
  --agent harbor_coder.agent:CoderAgentsAgent \
  --env harbor_coder.environment:CoderEnvironment \
  --ek template="$HARBOR_CODER_TEMPLATE" -k 2 -n 3 -o jobs --job-name main
```

Run again on another checkout (or with `--ak system_prompt=...`) under a
different `--job-name`, then:

```sh
uv tool run --from harbor python -m harbor_coder.compare \
  --baseline jobs/main --candidate jobs/branch
```

Set `HARBOR_CODER_SKIP_BUILD=1` to reuse an existing `.harbor-coder/coder`.

## Environment provider options

Pass with `--ek key=value` (or env var):

| Option | Env | Purpose |
| --- | --- | --- |
| `template` | `HARBOR_CODER_TEMPLATE` | Template name (required). Must accept `image`, `workdir`, `cpus`, `memory_mb` parameters. |
| `org` | `HARBOR_CODER_ORG` | Organization for `coder create`. |
| `registry` | `HARBOR_CODER_REGISTRY` | Push built images here when the provisioner's Docker host is not the local one. |
| `coder_binary` | | Path to the `coder` CLI baked into task images; defaults to the one on `PATH`. |

Capabilities: no network isolation (the agent must reach coderd), no GPU,
not mounted. Tasks using `network_mode = "no-network"` are rejected.

## Agent options

Pass with `--ak key=value`:

| Option | Purpose |
| --- | --- |
| `system_prompt` | Extra per-chat system prompt (`CreateChatRequest.system_prompt`). Appended to the deployment default, not a replacement. |
| `model_config_id` | Chat model config; defaults to the org default. |
| `reasoning_effort` | Passed through to the chat. |

The agent records `n_input_tokens`, `n_output_tokens`, `n_cache_tokens`,
`cost_usd` (from `/api/v2/chats/{id}/cost`) and writes the full transcript to
`agent/chat.json` in the trial directory.

## Adding tasks

Follow the [Harbor task format](https://harborframework.com/docs): `task.toml`,
`instruction.md`, `environment/Dockerfile`, `tests/test.sh` writing
`/logs/verifier/reward.txt`, and `solution/solve.sh` so `--agent oracle` can
validate the task without spending tokens:

```sh
harbor run -p scripts/harbor-coder/tasks --agent oracle \
  --env harbor_coder.environment:CoderEnvironment --ek template="$HARBOR_CODER_TEMPLATE"
```
