---
display_name: Docker + Chat Sandbox
description: Two-agent Docker template with a bubblewrap-sandboxed chat agent
icon: ../../../../site/static/icon/docker.png
maintainer_github: coder
tags: [docker, container, chat]
---

> **Experimental**: This template depends on the `-coderd-chat` agent
> naming convention, which is an internal PoC mechanism subject to
> change. Do not rely on this for production workloads.

# Docker + Chat Sandbox

This template provisions a workspace with two agents:

| Agent             | Purpose                                           | Visible in UI |
|-------------------|---------------------------------------------------|---------------|
| `dev`             | Regular development agent with code-server        | Yes           |
| `dev-coderd-chat` | AI chat agent running inside a bubblewrap sandbox | Yes           |

## How it works

The `dev` agent is a standard workspace agent with code-server and
full filesystem access. Users interact with it normally through the
dashboard, SSH, and Coder Connect.

The `dev-coderd-chat` agent is designated for AI chat sessions via the
`-coderd-chat` naming suffix. Chatd routes chat traffic to this agent
automatically. The dashboard and REST API still expose it like any other
agent, but this template treats it as a chatd-managed sandbox rather
than a normal user interaction surface.

## Bubblewrap sandbox

The chat agent's init script is wrapped with
[bubblewrap](https://github.com/containers/bubblewrap) so the **entire
agent process** runs inside a restricted mount namespace with **all
capabilities dropped**. Every child process the agent spawns (tool calls
via `sh -c`, SSH sessions) inherits the same restrictions.

The Coder agent hardcodes `sh -c` for tool call execution and ignores
the `SHELL` environment variable, so wrapping only the shell would be
ineffective. Wrapping the agent binary means the `/bin/bash`, `python3`,
or any other binary the model invokes is the one inside the read-only
namespace.

### Sandbox policy

- **Read-only root filesystem**: cannot install packages, modify system
  config, or tamper with binaries. Enforced by the kernel mount
  namespace, applies even to the root user.
- **Read-write /home/coder**: project files are editable (shared with
  the dev agent via a Docker volume).
- **Read-write /tmp**: scratch space (the agent binary downloads here
  during startup, tool calls can use it).
- **Shared /proc and /dev**: bind-mounted from the container so CLI
  tools and the agent work normally.
- **Outbound TCP allowlist**: before entering bwrap, the wrapper
  installs `iptables` and `ip6tables` OUTPUT rules that allow loopback,
  `ESTABLISHED,RELATED`, and new TCP connections only to the
  control-plane host and port used by the agent. All other outbound TCP
  is rejected over both IPv4 and IPv6.
- **Near-zero capabilities**: bwrap drops all Linux capabilities
  except `CAP_DAC_OVERRIDE` before exec'ing the agent. This prevents
  mount escape (`mount --bind`), ptrace, raw network access, and all
  other privileged operations. `DAC_OVERRIDE` is retained so the
  sandbox process (root) can read/write files owned by uid 1000
  (coder) on the shared home volume without changing ownership.

### How the capability lifecycle works

1. Docker starts the container as root with `CAP_SYS_ADMIN`,
   `CAP_NET_ADMIN`, and `CAP_DAC_OVERRIDE`.
2. The entrypoint runs `bwrap-agent`, which resolves the control-plane
   host and installs the outbound TCP allowlist with `iptables` and
   `ip6tables`.
3. bwrap creates the mount namespace using `CAP_SYS_ADMIN`.
4. bwrap drops all capabilities except `DAC_OVERRIDE`.
5. bwrap exec's the agent binary with only `DAC_OVERRIDE`.
6. All tool calls spawned by the agent inherit only `DAC_OVERRIDE`.

After step 4, the process cannot remount filesystems, change ownership,
ptrace other processes, or perform any other privileged operation. It
can read and write files regardless of Unix permissions, which is needed
because the shared home volume is owned by uid 1000 (coder) but the
sandbox runs as root.

### Limitations

- **No PID namespace isolation**: Docker's namespace setup conflicts
  with nested PID namespaces (`--unshare-pid`). Processes inside the
  sandbox can see other container processes via `/proc`.
- **No user namespace isolation**: Docker blocks nested user namespaces.
  The container runs as root uid 0, but with zero capabilities the
  effective privilege level is lower than an unprivileged user.
- **Only outbound TCP is filtered**: UDP, ICMP, and inbound traffic
  still follow Docker's normal container networking rules. DNS usually
  continues to work over UDP, but DNS-over-TCP is blocked unless it uses
  the control-plane endpoint.
- **IP resolution at startup**: the outbound allowlist resolves the
  control-plane hostname once with `getent ahostsv4` and, when IPv6 is
  enabled, `getent ahostsv6`. If those lookups fail, or if the endpoint
  later moves to a different IP, the chat container must restart to
  refresh the rules.
- **seccomp=unconfined**: Docker's default seccomp profile blocks
  `pivot_root`, which bwrap needs. A custom seccomp profile that allows
  only `pivot_root` and `mount` would be more restrictive.

Template authors can adjust the sandbox policy in `bwrap-agent.sh` by
adding `--bind` flags for additional writable paths.

## Usage

After starting `./scripts/develop.sh`, push this template:

```bash
cd examples/templates/x/docker-chat-sandbox
coder templates push docker-chat-sandbox \
  --var docker_socket="$(docker context inspect --format '{{ .Endpoints.docker.Host }}')"
```

Then create a workspace from it and start a chat session.
