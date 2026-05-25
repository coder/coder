# coder-local: local dogfood template

This template runs a Coder dogfood workspace as a Docker container on a
developer's own machine (typically a MacBook), provisioned via a user-scoped
provisioner. It is a simplified variant of the remote dogfood template in
`dogfood/coder/`.

## Prerequisites

- Docker Desktop (or Docker Engine) running on macOS or Linux
- Coder CLI installed and authenticated against your Coder deployment
- A user-scoped provisioner running on your machine (see below)

## Starting the provisioner

Run once in any terminal that will stay alive while you use the workspace:

```sh
coder provisioner start --tag scope=user
```

The workspace will be routed to your local provisioner via the `scope=user`
workspace tag.

## How it differs from the remote dogfood template

| Feature | Remote (`dogfood/coder`) | Local (`dogfood/coder-local`) |
|---|---|---|
| Provisioner | Cluster-scoped (GKE) | User-scoped (your machine) |
| Docker host | Remote TCP (Tailscale) | Inherits `DOCKER_HOST` from env |
| Container runtime | sysbox-runc | Default (runc) |
| Memory limit | 32768 MiB | None |
| Inner Docker daemon | Yes (/var/lib/docker volume) | No |
| Docker socket | None | Host socket bind-mounted |
| Devcontainer support | Yes | No |
| Regions / presets | Yes | No |
| Stop timeout | 300 s | 60 s |

## Volume persistence

Two Docker named volumes are created per workspace:

- `coder-<id>-home` mounted at `/home/coder/` - persists your home directory,
  Go module cache, build artifacts, etc.
- `coder-<id>-homebrew` mounted at `/home/linuxbrew/` - persists
  Homebrew formulae installed inside the container.

Volumes survive workspace stop/start cycles. They are only removed when the
workspace is deleted. The home volume carries a `lifecycle { ignore_changes =
all }` guard so renaming the workspace does not recreate it.

## Docker-outside-of-Docker

The host Docker socket (`/var/run/docker.sock`) is bind-mounted into the
container. Any `docker` commands run inside the workspace operate against
your host daemon. You are responsible for the containers and images that the
workspace creates on your machine.

## Known limitations

- **Laptop sleep.** The workspace agent loses connectivity whenever the laptop
  sleeps. Reconnect by waking the laptop; the agent will re-register
  automatically.
- **Host Docker access.** The Docker socket passthrough gives the workspace
  full access to your host Docker daemon. Treat the workspace with the same
  trust level as any process running directly on your machine.
- **Port conflicts.** Ports forwarded from the workspace share the host
  network namespace. Conflicts with services already bound on the host are
  possible.
- **No devcontainer support.** The inner Docker daemon required by devcontainers
  is not present. Use the remote dogfood template if you need devcontainer
  support.
- **amd64 image only.** The container image is built for amd64. On Apple
  Silicon, Docker Desktop runs it under Rosetta 2 emulation. arm64 native
  support is planned.
