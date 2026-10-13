# harbor-coder

A/B harness for Coder Agents system prompt changes, built on
[Harbor](https://github.com/harbor-framework/harbor). It answers: with this
prompt (or this branch), does the agent complete tasks better, cheaper, or
with different behavior than before?

- `harbor_coder.environment:CoderEnvironment`: a Harbor environment provider
  that runs each trial inside a Coder workspace.
- `harbor_coder.agent:CoderAgentsAgent`: a Harbor agent that drives Coder
  Agents (the coderd chat agent) against that workspace and records tokens,
  cost, and behavioral signals (steps, tool calls, whether it ran tests,
  whether it stopped to ask).
- `harbor_coder.prompt`: replace or reset the deployment system prompt, for
  prompt iteration without rebuilding coderd.
- `harbor_coder.bootstrap`: configures a fresh coderd (first user, Anthropic
  provider, chat model, workspace template) for eval runs.
- `harbor_coder.compare`: diffs two Harbor job directories into a Markdown
  verdict with per-task pass rates and behavior deltas.
- `tasks/`: a Harbor task suite with deterministic verifiers. Several tasks
  are designed to move on prompt behavior rather than raw capability:
  `scope-control` (fix one thing, touch nothing else), `verify-before-done`
  (the obvious patch is wrong; only running tests reveals it),
  `underspecified` (make an assumption and finish instead of asking).
- `template/`: the Coder template the environment provider creates
  workspaces from.

The GitHub workflow `eval/coder-agents` (`.github/workflows/eval-coder-agents.yaml`)
builds coderd from `main` and from a candidate ref, runs the suite against
each, and posts the comparison. Trials spend real model tokens, so it never
runs automatically.

- **On a PR:** comment `/coder-agents-eval`. The PR branch is the candidate,
  `main` is the baseline, and the comparison is posted back as a PR comment.
  Optional args on the same line: `model=`, `attempts=`, `concurrency=`,
  `baseline=` (for example `/coder-agents-eval model=claude-sonnet-4-5 attempts=3`).
  The commenter needs push access and the PR must be from this repository.
- **Manually:** `workflow_dispatch` with the same options; the result goes to
  the step summary. Set `candidate_ref` equal to `baseline_ref` once to
  measure the noise floor before trusting small deltas.

Model calls go through dogfood's AI gateway, authenticated with a Coder API
key (`HARBOR_CODER_UPSTREAM_TOKEN`; the `EVAL_CODER_SESSION_TOKEN` secret in
CI). No provider API key is involved, and spend is attributed through the
gateway like any other Coder Agents traffic.

## Two ways to A/B a prompt

**Branch vs main (the workflow).** Change `DefaultSystemPrompt` in
`coderd/x/chatd/prompt.go` on a branch and dispatch the workflow. Each arm
builds its own coderd, so anything the branch changes (prompt assembly,
tools, model handling) is part of the comparison.

**Prompt text vs prompt text (local, fast).** Against one running coderd,
swap the deployment prompt between runs:

```sh
uv tool run --from harbor python -m harbor_coder.prompt --reset
harbor run ... --job-name baseline
uv tool run --from harbor python -m harbor_coder.prompt --file candidate-prompt.md
harbor run ... --job-name candidate
uv tool run --from harbor python -m harbor_coder.compare --baseline jobs/baseline --candidate jobs/candidate
```

`--file` replaces the built-in default (`include_default_system_prompt=false`);
`--append-file` layers on top of it. Chats snapshot the prompt at creation, so
apply it before each run. This loop takes about two minutes per arm for the
six-task suite on haiku.

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
export HARBOR_CODER_UPSTREAM_TOKEN=$(coder tokens create --lifetime 8h)  # dogfood AI gateway auth
# HARBOR_CODER_UPSTREAM_URL overrides the default dogfood gateway
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
The binary is built with `CGO_ENABLED=0` so it runs inside any task image
regardless of that image's glibc.

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
`cost_usd` (from `/api/v2/chats/{id}/cost`), behavioral metadata (`n_turns`,
`n_tool_calls`, `n_exec`, `n_edit`, `n_tool_errors`, `ran_tests`,
`asked_question`, `edited_paths`, `agent_seconds`), and writes the full
transcript to `agent/chat.json` in the trial directory.

## Reading the comparison

Pass rate moves last. On a six-task suite one flipped trial is a 0.08 swing,
so the verdict carries a small-sample caveat until the suite is larger. The
behavior table is usually where a prompt change shows first: fewer steps and
tool errors at equal pass rate is a real improvement; a higher "ran tests"
rate with no pass change means the prompt bought verification discipline the
current tasks are too easy to reward. Run main-vs-main once to see how much
these numbers move on their own.

## Adding tasks

The suite only measures what its tasks reward. To test a prompt change, add a
task where the old prompt plausibly fails and the new one plausibly passes,
with a verifier that checks the end state (files, test results) and never
the transcript. Follow the [Harbor task format](https://harborframework.com/docs):
`task.toml`, `instruction.md`, `environment/Dockerfile`, `tests/test.sh`
writing `/logs/verifier/reward.txt`, and `solution/solve.sh` so
`--agent oracle` can validate the task without spending tokens:

```sh
harbor run -p scripts/harbor-coder/tasks --agent oracle \
  --env harbor_coder.environment:CoderEnvironment --ek template="$HARBOR_CODER_TEMPLATE"
```
