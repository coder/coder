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
each arm builds its own coderd (main vs the PR), runs `terminal-bench-sample@2.0`
with Coder Agents, and the comparison is posted on the PR. The system prompt
is Go code, so a prompt change is a branch and the unit of comparison is a
build. Runs cost about $2 per arm on haiku; one flipped trial moves the mean
by 0.07, so treat small deltas as noise.

## Local

```sh
uv tool install --with-editable scripts/harbor-coder harbor
export HARBOR_CODER_UPSTREAM_TOKEN=$(coder tokens create --lifetime 8h)   # dogfood gateway auth
./scripts/harbor-coder/serve-coderd.sh .harbor-coder/env
set -a; . .harbor-coder/env; set +a; export PATH="$PWD/.harbor-coder:$PATH"
harbor run -d terminal-bench-sample@2.0 -x 'qemu-*' -x build-cython-ext \
  --agent harbor_coder.agent:CoderAgentsAgent --env harbor_coder.environment:CoderEnvironment \
  -k 2 -n 4 -o jobs --job-name main
```

Rebuild from the other branch, rerun under another `--job-name`, then
`python -m harbor_coder.compare --baseline jobs/main --candidate jobs/branch`.
`--agent oracle` validates tasks without spending tokens. Any registry dataset
works if its image has `bash` and `tar`, runs as root, and does not need
`no-network` or a GPU.
