# harbor-coder

A/B Coder Agents between a branch and `main` using
[Harbor](https://github.com/harbor-framework/harbor) and Terminal-Bench 2.0.
Run it manually from a dogfood workspace; it is not wired into CI.

- `eval.sh`: the whole A/B. Builds coderd from the baseline and from the
  current checkout, runs Terminal-Bench 2.0 against each with Coder Agents,
  and prints the comparison.
- `harbor_coder.environment:CoderEnvironment`: Harbor environment provider
  that runs each trial in a Coder workspace (task image plus the `coder`
  binary, created from `template/`, exec over `coder ssh`).
- `harbor_coder.agent:CoderAgentsAgent`: Harbor agent that binds a chat to
  that workspace, waits for the turn to finish, and records tokens and cost.
- `harbor_coder.bootstrap`: first user, AI provider (dogfood's gateway), chat
  model, template push on a fresh coderd. Called by `serve-coderd.sh`.
- `harbor_coder.compare`: pass rate per task, cost, and a paired-bootstrap
  verdict for two job directories.

The system prompt is Go code, so a prompt change is a branch and the unit of
comparison is a coderd build. Nothing is injected from outside.

## Run

```sh
# Once: install Harbor into .harbor-coder/venv from requirements.lock with
# --require-hashes, so every dependency (litellm included) is pinned by sha256.
./scripts/harbor-coder/install.sh
export PATH="$PWD/.harbor-coder/venv/bin:$PATH"

# Dogfood gateway auth. The gateway ignores token scopes, so keep it narrow.
export HARBOR_CODER_UPSTREAM_TOKEN=$(coder tokens create --lifetime 8h --scope user:read_personal)

git checkout my-prompt-change
./scripts/harbor-coder/eval.sh
```

`eval.sh` builds coderd twice: from the branch's merge base with `main`
(in a temporary git worktree, so later `main` changes are not attributed to
your branch) and from the current checkout, including uncommitted changes.
Each arm gets a fresh embedded Postgres and runs 74 of Terminal-Bench 2.0's
89 tasks: 3 are broken upstream, 9 are 20 to 60 minute ML and emulation
tasks the prompt cannot affect, and 3 security tasks are always refused by
Anthropic's content policy. Results land in `jobs/baseline`, `jobs/candidate`,
and `jobs/report.md`.

`--baseline <ref>` overrides the baseline. `--baseline HEAD` runs the same
build twice, which is the noise floor; do that once before trusting a small
delta. Defaults are `claude-sonnet-5-5`, 2 attempts per task, concurrency 5
(chatd's unlicensed cap on concurrent chats); override with
`HARBOR_CODER_MODEL`, `HARBOR_ATTEMPTS`, `HARBOR_CONCURRENCY`,
`HARBOR_DATASET`, `HARBOR_PORT`, `HARBOR_JOBS_DIR`. Measured: about $23 and
90 minutes per arm at these defaults, so roughly $47 and 3 hours for a full
A/B; the complete 86 tasks on `main` scored 73% at $31.54 and 3.4h per arm.

`compare` calls a change better or worse only when a paired bootstrap 95%
interval over tasks excludes zero.

## Notes

- `--agent oracle` in place of the Coder Agents agent validates tasks without
  spending tokens.
- Any Harbor registry dataset works if its image has `bash` and `tar`, runs as
  root, and does not need `no-network` or a GPU.
- Each Terminal-Bench image is pulled and re-layered per task; a full arm
  churns ~100GB of Docker disk. `eval.sh` prunes images between arms.
- `.harbor-coder/` holds the built binary, embedded Postgres, logs, and the
  venv; it is gitignored.
