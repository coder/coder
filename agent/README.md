```
   ___          _                _                    _
  / __\___   __| | ___ _ __    / \   __ _  ___ _ __ | |_
 / /  / _ \ / _` |/ _ \ '__|  / _ \ / _` |/ _ \ '_ \| __|
/ /__| (_) | (_| |  __/ |    / ___ \ (_| |  __/ | | | |_
\____/\___/ \__,_|\___|_|   /_/   \_\__, |\___|_| |_|\__|
                                     |___/
```

# agent

The `agent` package implements the Coder workspace agent — a long-running
process that runs inside every Coder workspace. It is the primary bridge
between the Coder control plane and the workspace environment.

## What It Does

The agent connects back to the Coder server over a WireGuard-based
[Tailnet](https://coder.com/docs/networking) tunnel and exposes workspace
functionality:

- **SSH** — Full SSH server for terminal access (`agentssh`)
- **Port Forwarding** — Detects listening ports and enables forwarding
- **App Health** — Monitors and reports workspace app health
- **Startup Scripts** — Executes agent lifecycle scripts (`agentscripts`)
- **Metrics & Stats** — Collects and reports Prometheus metrics and session stats
- **Reconnecting PTY** — Persistent terminal sessions that survive disconnects (`reconnectingpty`)
- **Dev Containers** — Manages devcontainer lifecycle (`agentcontainers`)
- **File Transfer** — Provides file access to the workspace (`agentfiles`)

## Subpackages

| Package | Description |
|---|---|
| `agentcontainers` | Dev container detection, lifecycle, and API |
| `agentexec` | Process execution abstraction |
| `agentfiles` | File transfer service |
| `agentproc` | Process listing and management |
| `agentrsa` | RSA key generation for SSH host keys |
| `agentscripts` | Startup/shutdown script runner |
| `agentsocket` | Unix socket server for local IPC |
| `agentssh` | Built-in SSH server |
| `agenttest` | Test helpers and fake agent construction |
| `boundarylogproxy` | Log boundary proxy for sub-agents |
| `filefinder` | File search utilities |
| `immortalstreams` | Reconnectable stream wrappers |
| `proto` | Protobuf definitions for agent ↔ server RPC |
| `reaper` | Zombie process reaper (PID 1 behavior) |
| `reconnectingpty` | Persistent PTY sessions |
| `unit` | systemd integration |
| `usershell` | Detects the user's default shell |

## Architecture

```
┌──────────────────────────────────────────┐
│              Coder Server                │
│         (coderd + Tailnet DERP)          │
└──────────────┬───────────────────────────┘
               │  WireGuard / DRPC
               ▼
┌──────────────────────────────────────────┐
│            Workspace Agent               │
│                                          │
│  ┌──────────┐ ┌────────┐ ┌───────────┐  │
│  │   SSH    │ │  PTY   │ │  Scripts  │  │
│  │  Server  │ │ Server │ │  Runner   │  │
│  └──────────┘ └────────┘ └───────────┘  │
│  ┌──────────┐ ┌────────┐ ┌───────────┐  │
│  │  Port    │ │  App   │ │   Dev     │  │
│  │ Forward  │ │ Health │ │ Container │  │
│  └──────────┘ └────────┘ └───────────┘  │
│  ┌──────────┐ ┌────────┐ ┌───────────┐  │
│  │ Metrics  │ │ Files  │ │  Reaper   │  │
│  └──────────┘ └────────┘ └───────────┘  │
└──────────────────────────────────────────┘
```
