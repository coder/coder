# harbor-coder

A/B Coder Agents between a branch and `main` using
[Harbor](https://github.com/harbor-framework/harbor) and Terminal-Bench.

- `harbor_coder.environment:CoderEnvironment`: Harbor environment provider
  that runs each trial in a Coder workspace (task image plus the `coder`
  binary, created from `template/`, exec over `coder ssh`).
- `harbor_coder.agent:CoderAgentsAgent`: Harbor agent that binds a chat to
  that workspace, waits for the turn to finish, and records tokens and cost.
- `harbor_coder.bootstrap`: first user, AI provider (dogfood's gateway), chat
  model, template push on a fresh coderd.
- `harbor_coder.compare`: pass rate per task and cost for two job directories.

Comment `/coder-agents-eval` on a PR to run `.github/workflows/eval-coder-agents.yaml`:
each arm builds its own coderd (main vs the PR), runs `terminal-bench@2.0`
(77 of 89 tasks: 3 are broken upstream and 9 are 20 to 60 minute ML and
emulation tasks the prompt cannot affect) with Coder Agents on
`claude-sonnet-5-5`, and the comparison is posted on the PR. The baseline is
the commit the PR branched from, not the `main` tip, so later `main` changes
are not attributed to the PR. The system prompt is Go code, so a prompt
change is a branch and the unit of comparison is a build. The verdict calls
a change better or worse only when a paired bootstrap 95% interval over
tasks excludes zero.

Measured on the 77-task set at concurrency 5 (chatd's unlicensed cap on
concurrent chats): about $23 and 90 minutes per arm
([run](https://github.com/coder/coder/actions/runs/40423882171)). One
`/coder-agents-eval` runs two arms in parallel, so about $47 of gateway
spend and two 8-core runners for roughly 1.5h. The full 86 tasks on `main`
scored 73% at two attempts, $31.54 and 3.4h for one arm.
Dataset, model, attempts, concurrency, and the Harbor version are the `env`
block at the top of the workflow.

## Local

```sh
uv tool install --with-editable scripts/harbor-coder harbor==0.24.0
# dogfood gateway auth; the gateway ignores token scopes, so keep it narrow
export HARBOR_CODER_UPSTREAM_TOKEN=$(coder tokens create --lifetime 8h --scope user:read_personal)
export HARBOR_CODER_MODEL=claude-sonnet-5-5
./scripts/harbor-coder/serve-coderd.sh .harbor-coder/env
set -a; . .harbor-coder/env; set +a; export PATH="$PWD/.harbor-coder:$PATH"
# add the -x list from HARBOR_EXCLUDE in the workflow to match CI
harbor run -d terminal-bench@2.0 -x 'qemu-*' -x build-cython-ext \
  --agent harbor_coder.agent:CoderAgentsAgent --env harbor_coder.environment:CoderEnvironment \
  -k 2 -n 4 -o jobs --job-name main
```

Stop the first coderd (`kill $(cat .harbor-coder/coderd.pid)`), check out the
other branch, rerun `serve-coderd.sh` and Harbor under another `--job-name`, then
`python -m harbor_coder.compare --baseline jobs/main --candidate jobs/branch`.
`serve-coderd.sh` refuses to start while something answers on its port, and
task images are tagged with the `coder` binary's hash, so the second run cannot
silently reuse the first build.
`--agent oracle` validates tasks without spending tokens. Any registry dataset
works if its image has `bash` and `tar`, runs as root, and does not need
`no-network` or a GPU.
