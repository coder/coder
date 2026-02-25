# Coder Capture — AI Session Tracking Module

A Terraform module that installs and enables `coder-capture` inside a Coder
workspace. It automatically detects AI coding-tool sessions (Claude Code,
Gemini CLI, Codex, Cursor, Aider, and others) via git hooks, captures session
metadata, and reports activity back to the Coder control plane. Template admins
can drop this module into any workspace template with a single `module` block —
no changes to the underlying Docker image are required if the binary is
pre-installed.

---

## Quick Start

```hcl
module "coder_capture" {
  source   = "./modules/coder-capture"
  agent_id = coder_agent.main.id
}
```

With optional flags:

```hcl
module "coder_capture" {
  source     = "./modules/coder-capture"
  agent_id   = coder_agent.main.id
  no_trailer = true               # suppress commit-message trailers
  log_dir    = "/var/log/capture" # store logs outside $HOME
}
```

> **Prerequisite**: The `coder-capture` binary must be available either in the
> workspace's `$PATH` (e.g. baked into the Docker image) or at
> `~/.coder-capture/bin/coder-capture`. The startup script copies it into place
> automatically on first start if found in `$PATH`.

---

## Variables

| Name         | Type     | Default | Description                                                                 |
|--------------|----------|---------|-----------------------------------------------------------------------------|
| `agent_id`   | `string` | —       | **Required.** The ID of the Coder agent to attach the capture script to.   |
| `no_trailer` | `bool`   | `false` | If `true`, suppress the `Coder-Session: …` trailer in git commit messages. |
| `log_dir`    | `string` | `""`    | Custom directory for capture logs. Defaults to `~/coder-capture-logs/`.     |

---

## How It Works

### 1. Binary Installation

On workspace start the `run.sh` script checks for
`~/.coder-capture/bin/coder-capture`. If absent, it looks for the binary in
`$PATH` and copies it into place. If the binary is not found anywhere, the
script prints a helpful warning and exits cleanly — the workspace still starts
normally.

### 2. Session Detection via Git Hooks

`coder-capture enable` installs `post-commit` and `prepare-commit-msg` hooks
into the user's global git hook directory (`core.hooksPath`). These hooks fire
on every `git commit` and inspect the process tree to detect which AI tool is
active.

### 3. AI Tool Detection

Each hook checks parent processes and environment variables for well-known AI
coding tools. When a match is found, the session ID, tool name, model, and
timestamp are recorded.

### 4. Session Trailer Injection

By default, `coder-capture` appends a `Coder-Session:` trailer to each commit
message so that sessions can be correlated with commits in post-hoc analysis.
Set `no_trailer = true` to disable this behaviour.

### 5. API Reporting

Session events are batched and sent to the Coder API using the workspace agent
token (`CODER_AGENT_TOKEN`). Reports appear in the workspace activity feed and
can be aggregated across the deployment via the admin dashboard or the Coder
API.

---

## Supported AI Tools

| Tool            | Detection Method                              | Notes                              |
|-----------------|-----------------------------------------------|------------------------------------|
| **Claude Code** | `CLAUDE_SESSION_ID` env var + process name    | Full session & model reporting     |
| **Gemini CLI**  | `GEMINI_SESSION` env var + `gemini` process   | Session ID captured from env       |
| **OpenAI Codex**| `CODEX_SESSION_ID` env var + process tree     | Requires Codex CLI ≥ 1.4           |
| **Cursor**      | `CURSOR_SESSION_ID` env var + window title    | Detected when committing from IDE  |
| **Aider**       | `.aider*` commit message patterns + env       | Trailer and log-file based         |

> Detection is best-effort. Tools that commit via a subprocess may not be
> detected if they clear the environment before spawning `git`.

---

## AI Bridge Integration

`coder-capture` integrates with the **Coder AI Bridge** to provide richer
session context. When AI Bridge is running in the workspace, `coder-capture`
reads structured session data from the Bridge's local socket instead of
relying solely on environment variables and process inspection.

To enable AI Bridge support, add the `ai-bridge` module alongside this one:

```hcl
module "ai_bridge" {
  source   = "./modules/ai-bridge"
  agent_id = coder_agent.main.id
}

module "coder_capture" {
  source   = "./modules/coder-capture"
  agent_id = coder_agent.main.id
}
```

The two modules discover each other automatically via a shared local socket at
`/tmp/coder-ai-bridge.sock`; no extra configuration is needed.

### Without AI Bridge vs. With AI Bridge

| Capability                        | Without AI Bridge          | With AI Bridge                        |
|-----------------------------------|----------------------------|---------------------------------------|
| Session detection                 | Environment + process tree | Structured socket protocol            |
| Model identification              | Best-effort from env vars  | Exact model string from tool          |
| Token / cost tracking             | ✗                          | ✓ (where tool exposes usage)          |
| Multi-agent session correlation   | ✗                          | ✓                                     |
| Idle vs. active session tracking  | ✗                          | ✓                                     |
| Reliability on env-clearing tools | Low                        | High                                  |

---

## Resources Created

| Resource                    | Description                                      |
|-----------------------------|--------------------------------------------------|
| `coder_script.coder_capture`| Startup script that installs and enables capture |

No persistent storage or network resources are provisioned by this module.

---

## Troubleshooting

**Binary not found warning on start**
: Bake `coder-capture` into your Docker image:
  ```dockerfile
  COPY --chown=root:root coder-capture /usr/local/bin/coder-capture
  RUN chmod +x /usr/local/bin/coder-capture
  ```

**Commits not being attributed**
: Check that the git global hooks path is not overridden in the workspace.
  Run `git config --global core.hooksPath` to inspect the current value.

**Logs location**
: Default log directory is `~/coder-capture-logs/`. Override with `log_dir`.
  Each session creates a timestamped file: `capture-<timestamp>.jsonl`.
