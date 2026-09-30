# Homelab Coder deployment — agent orientation

You are operating against Trillium's homelab Coder deployment, not a fresh
install. Read this file first, then inspect live state before changing
anything. Canonical procedures live in `.agents/skills/coder-homelab/SKILL.md`;
upstream first-time setup lives in `.agents/skills/coder-setup/SKILL.md`.

## Deployment (verified live 2026-09-30)

- **Control plane:** Docker container `coder` on **lnx**
  (`100.81.88.113`, MagicDNS `lnx.hippo-tilapia.ts.net`).
  Image `coder-keep:pr15-pr16-batch`, restart policy `unless-stopped`.
- **Access URL:** `http://lnx.hippo-tilapia.ts.net:7080` (tailnet-only, no
  public ingress). `CODER_HTTP_ADDRESS=0.0.0.0:7080`.
- **Version:** `v2.37.1-devel+337f49d6cd` — a development build. There is no
  downloadable CLI for it (`/bin/coder-darwin-arm64` 404s); closest clients
  are brew stable (2.36.6) and coder/coder tap (2.37.3). Version-mismatch
  warnings are expected, not failures. Do not hand-copy binaries to chase an
  exact match. Exception: the Linux agent binary IS hand-seeded from the
  running build (`docker cp coder:/opt/coder` into
  `/home/coder/.cache/coder/site/orig/bin/coder-linux-amd64`); without it
  `/bin/coder-linux-amd64` 404s and no workspace agent can start. Re-seed
  after every recreate (verified 2026-09-30: HEAD 200, ELF x86-64).
- **Mounts:** `/var/lib/coder:/var/lib/coder`, `/var/run/docker.sock`
  (workspaces are sibling Docker containers on lnx).
- **Database:** built-in PostgreSQL, data at
  `/home/coder/.config/coderv2/postgres` inside the container (host
  `/var/lib/coder` bind is empty). No automated backup exists (no crontab
  on lnx as of 2026-09-30) — treat the container as non-deletable;
  snapshot with `docker commit coder coder-keep:<reason>-YYYYMMDD`
  before touching the deployment (taken 2026-09-30:
  `coder-keep:pre-bin-fix-20260930`). See open questions below.
- **Provisioning:** built-in provisioner daemons (`scope=organization`,
  key `built-in`); no separate provisionerd on this single host.
- **CLI auth (this machine):** `coder` CLI is logged in as `admin`.
  Read-only commands (`templates list`, `list`, `show <workspace>`,
  `templates pull`) work now; anything mutating needs the task brief.

## Machine roles

| Machine | Role | Notes |
|---|---|---|
| lnx (100.81.88.113) | coderd host + workspace host | Docker workspaces run here as sibling containers |
| mini1 (100.102.238.x) | Apple fleet, CLI authed | File-based session (`CODER_USE_KEYRING=false` in `~/.zshrc`) |
| mini2 (100.111.197.110) | Apple fleet, CLI authed; separate local Coder v2.35.3 on :3000 (2026-09-10, stale) | Do not assume :3000 state without re-checking |
| mini3 (100.86.9.58) | Apple fleet, CLI authed | SSH user `2020mini_2` |
| MacBook | Operator machine, CLI authed via keychain | Coder Desktop optional, not installed |
| iPhone/iPad | Browser-only client | Dashboard over Tailscale; web terminal + code-server app |

## Architecture layers (change the right one)

`coder-azvc2` summary: CLI/UI -> **coderd control plane** (API, state,
Postgres) -> **templates** (Terraform blueprints) -> **provisionerd**
(Terraform execution) -> **infrastructure** (Docker containers/volumes on
lnx) -> **workspace agent** (terminal/SSH/apps). The AI agent loop is
control-plane-side and separate from the workspace agent.

## Live inventory (verified 2026-09-24)

- **Templates:** `docker-test` (org `coder`), version `auspicious_cole75` —
  upstream `docker` starter plus `coder_access_host` bridge-DNS fix.
  Canonical source: `homelab/templates/docker-test/` in this fork; working
  copy pushed from lnx: `/home/trillium/coder-templates/docker-test/`.
- **Template:** `bootstrap-admin` (org `coder`), version
  `encouraging_yost24` (pushed 2026-09-24) — same Docker base and DNS fix
  as `docker-test`, plus admin startup script (git, gh, terraform 1.9.8,
  Coder CLI, optional `config_repo_url` checkout). Canonical source:
  `homelab/templates/bootstrap-admin/`; host working copy:
  `/home/trillium/coder-templates/bootstrap-admin/` (md5-verified in sync
  at push time). No `coder` CLI on lnx, so pushes run from an authed Mac
  with identical content, not from the host path.
- **Workspaces:** `trillium-admin/coder-inspect` and
  `trillium-admin/deepseek-test` — both Started and agent-healthy — plus
  `trillium-admin/bootstrap-admin` (first admin workspace, Started and
  agent-healthy since 2026-09-24; tooling verified: git 2.55, gh 2.46,
  terraform 1.9.8, coder CLI; `~/config` absent because no
  `config_repo_url` was passed).
- **Users:** `admin`, `trillium`, `trillium-admin`, `trillium-web` (all active).
- **Other lnx containers (do not touch):** `pihole`, `open_crm-try-*`,
  `coder-admin-proof1`.

## Constraints (non-negotiable)

1. **Community features preferred.** Never opt the deployment into trials
   or Premium signup; External Workspaces enrollment is NOT approved
   (Premium gate unconfirmed — verify license page first).
2. **Existing Macs are persistent infrastructure.** Lifecycle ops never
   destroy, rebuild, or re-image them. Deleting a workspace removes only
   Coder records/tokens where applicable — confirm scope first.
3. **Inspect before mutate.** Determine what is running, what each
   component does, which templates/workspaces exist, and which layer is
   being changed before making any change.
4. **Secrets never land in git.** Session tokens, admin passwords, and API
   keys live host-only (mode 600), e.g. `/home/trillium/.coder-admin-token`.
5. **Fork-only.** PRs target `trillium/coder`, never upstream `coder/coder`.

## Runbook locations

- Operational walkthrough: `.agents/skills/coder-homelab/SKILL.md`
- Upstream first-time setup (fresh installs only): `.agents/skills/coder-setup/SKILL.md`
- Template sources + sync procedure: `homelab/templates/<name>/README.md`
- Template deltas vs upstream starter: `diff` the vendored `main.tf`
  against `examples/templates/docker/main.tf`
- Prior deployment knowledge: brain `brain-89mf1` (mini2 local),
  `brain-3vp70` (access-URL root cause), `brain-7sc9i` (agents architecture),
  `brain-azvc2` (device integration), task `task-i4umg` (fleet rollout record)

## Open questions (verify before relying on them)

- Exact Postgres data path and a backup/restore procedure (`/var/lib/coder`
  listing was inconclusive; no backups scheduled).
- mini2 `:3000` local instance current state (unreachable from here on 2026-09-24).
- External Workspaces license/entitlement status on lnx.

## Maintaining this file

Keep only deployment facts nearly every session needs; prefer pointers over
copies. Update the verified-live date whenever state is re-confirmed, and
move resolved questions into facts (or the skill). Homelab glue stays under
`homelab/` and `.agents/skills/coder-*/` so fork diffs stay cherry-pickable
and upstream files stay untouched.
