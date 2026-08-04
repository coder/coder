---
display_name: Linux bubblewrap subagent (experimental)
description: Human Docker workspace with a nested Coder subagent isolated by the vetted bubblewrap reference driver.
tags: [docker, experimental, isolation]
---

# Linux bubblewrap nested subagent (experimental)

This template is an experimental proof-of-concept reference for execution
isolation. It is not a published example, it is not supported for production
use, and it can change or disappear without notice.

## Purpose

The template declares one human workspace and one nested isolated child
agent:

- a normal Linux Docker workspace with a top-level `coder_agent`, whose
  working directory is `/home/coder/project`;
- one `coder_subagent_execution` that Coder launches through the checked-in
  bubblewrap reference driver at `drivers/bwrap.sh`;
- a child agent whose only writable host directory is
  `/home/coder/project`, mounted inside the sandbox at
  `/workspace/project`;
- a child-side BusyBox `httpd` workload plus an owner-shared `coder_app`, so
  the sandbox has a visible workload and the shared directory has visible
  contents.

Everything else the child sees is created by the driver: an empty private
root, a minimal BusyBox `/bin`, generated account files, a private home,
temporary, and runtime directory, and the Coder binary read-only.

## Prerequisites

- A Coder deployment built from a server branch that implements
  `coder_subagent_execution` (declaration, persistence, the pre-created
  isolated child, and the parent-side `agent/subagentexec` manager), and a
  `terraform-provider-coder` branch that ships the
  `coder_subagent_execution` resource. Neither is in a release yet, so the
  provider version in `main.tf` is deliberately unpinned.
- Linux host with Docker, plus `host.docker.internal` reachability for a
  local deployment (the template already adds the `host-gateway` host
  entry).
- Unprivileged user namespaces permitted on the host kernel. bubblewrap
  builds the sandbox from an unprivileged user namespace; the container adds
  no capabilities.
- `seccomp=unconfined` and `apparmor=unconfined` on the workspace container.
  Docker's default seccomp profile blocks the `mount` and `pivot_root` calls
  bubblewrap issues inside its own user namespace, and on AppArmor hosts the
  `docker-default` profile denies unprivileged user namespace creation. Both
  are declared explicitly in `main.tf`. They relax the parent container's
  confinement only; they are not part of the boundary the driver builds
  around the child.
- A statically linked Coder binary on the parent agent. The manager passes
  the parent agent's own executable to the driver, and the driver binds it
  into a sandbox with no host library directories. A dynamically linked,
  CGO-enabled, or BoringCrypto/FIPS Coder build cannot execute inside the
  minimal root.
- Host packages in the workspace image: `bubblewrap`, `jq`,
  `busybox-static`, and `ca-certificates`. The provided `Dockerfile`
  installs all four and fails the build if the resolved BusyBox is not
  static.
- No Docker socket and no parent home directory are mounted anywhere in this
  template. Only `/home/coder/project` is a persistent volume; the parent's
  home directory is container-local.

## Demo steps

1. Push the template from this directory:

   ```console
   coder templates push linux-bwrap-subagent -d .
   ```

   The `docker_image` resource builds the reference image from the
   `Dockerfile` in this directory.

2. Create a workspace from the template and wait for the parent agent to
   connect.

3. In the dashboard, open the workspace and confirm:
   - the parent agent `main` is connected;
   - a nested child agent `sandbox` appears underneath it and reports
     execution isolation;
   - the `Sandbox Driver` metadata item reads `running`;
   - the child's launcher logs are available on the parent agent.

4. Open the child agent's web terminal. This is the ordinary nested-agent
   terminal; the template does not declare a terminal app.

5. Open the owner-shared `Sandbox project page` app on the child. It serves
   `/workspace/project/index.html`, which the child's startup script wrote.

6. From the parent workspace terminal, confirm the shared directory really
   is shared:

   ```console
   ls -l /home/coder/project
   cat /home/coder/project/sandbox-marker.txt
   echo "hello from the human" > /home/coder/project/from-parent.txt
   ```

   Then read `/workspace/project/from-parent.txt` from the child terminal.

## Isolation probes from the child terminal

Run these in the child's web terminal. The sandbox userland is BusyBox, so
these are all BusyBox applets.

```sh
# The one shared directory is present and writable.
touch /workspace/project/from-child.txt && ls -l /workspace/project

# The parent's home, dotfiles, SSH directory, and the Docker socket are
# absent: the child's /home/coder is its own private directory, and no host
# root is bound.
ls -a /home/coder                 # private home, not the human's home
ls /home/coder/.ssh               # No such file or directory
ls /var/run/docker.sock           # No such file or directory (no /var at all)

# No host system directories exist.
ls /usr                           # No such file or directory
ls /lib                           # No such file or directory

# The private home, temporary, runtime, proc, and dev directories exist.
ls -d /home/coder /tmp /run/user/1000 /proc /dev

# Only sandbox processes are visible: the pid namespace is unshared.
ps

# The child runs as an unprivileged user with no capabilities.
id
```

## Support claim

Only the checked-in `drivers/bwrap.sh` in this directory is the vetted
reference driver for the PoC. The sandbox properties described above are
claimed for that script, invoked by the Coder-owned manager, on a host that
meets the prerequisites above.

Custom drivers speak the same protocol (`run` and `cleanup`, protocol
version 1, one JSON input document), and they are **trusted, not certified**.
Coder cannot prove sandbox properties for an arbitrary script: a custom
driver is as trusted as the template that ships it.

## Limitations

- **Network is shared.** The driver deliberately does not unshare the
  network namespace, because the child agent must reach the deployment.
  There is no network egress isolation of any kind.
- **No Agent Identity.** The child has a workspace-agent token and nothing
  else. It has no agent principal, no agent-owned credentials, and no Git or
  external-auth material. Human credentials are simply absent, which is a
  "no principal" state rather than an identity model.
- **`startup_timeout` and `restart_policy` are declared but not enforced.**
  Both values are persisted and delivered to the parent agent, and the
  manager does not yet implement readiness timeouts or automatic restart.
  `restart_policy = "never"` therefore describes the behavior you actually
  get today; `on-failure` is the schema default and would not change current
  behavior either. A driver that exits is not restarted until the next
  reconciliation trigger.
- **`RUNNING` means the driver process started**, not that the child agent
  is ready or that the sandbox is healthy.
- **Minimal BusyBox userland.** The child has one static BusyBox and the
  applets the driver exposes. There is no package manager, no compiler
  toolchain, no `bash`, and no container runtime. Child-side scripts must be
  POSIX shell using those applets.
- **The shared project directory is fully shared, by design.** Anything the
  human owner places inside `/home/coder/project`, including keys, tokens,
  sockets, or checkouts, is readable and writable by the child. The contract
  is about paths, not contents: keeping something away from the child means
  keeping it out of the project directory.
- **Only the project directory persists.** The parent's home directory and
  everything private to the sandbox are container-local and are recreated on
  rebuild.
- **A dynamic, CGO-enabled, or BoringCrypto Coder binary is unsupported**
  by the minimal root, as described in the prerequisites.

## Troubleshooting

**`bwrap: setting up uid map: Permission denied`, `No permissions to create
new namespace`, or `Creating new namespace failed`**

The host or container is denying unprivileged user namespace creation. Check,
in order:

```console
# On the Docker host.
cat /proc/sys/user/max_user_namespaces          # must be > 0
sysctl kernel.unprivileged_userns_clone         # if present, must be 1
sysctl kernel.apparmor_restrict_unprivileged_userns  # if present, 1 blocks bwrap
```

Ubuntu 23.10 and later restrict unprivileged user namespaces through
AppArmor, which is why the container declares `apparmor=unconfined`. If your
Docker daemon or host policy forbids relaxing seccomp or AppArmor, this
template cannot run there.

`bwrap: setting up uid map: Permission denied` is a different, later failure:
the user namespace was created and the kernel then refused the identity
mapping. In practice that means the workspace container is already nested
inside a user namespace whose ranges it cannot subdivide, which is what a
Docker daemon with `userns-remap`, a rootless Docker daemon, or a daemon
running inside another AppArmor-confined container looks like. Check
`cat /proc/self/uid_map` inside the workspace: a line other than
`0 0 4294967295` means the container is remapped. Run this template on a
Docker daemon without user namespace remapping.

**`bwrap: pivot_root: Operation not permitted` or a failure during mount
setup**

Docker's default seccomp profile is still in effect. Confirm the container
was created with the `security_opts` this template declares.

**The child agent never connects, and the launcher log shows an exec format
or loader error**

The Coder binary the manager passed is not statically linked. Use a Coder
build produced with `CGO_ENABLED=0` (the standard release binaries), not a
CGO or FIPS/BoringCrypto build.

**`bwrap driver: required tool not found on PATH: busybox` (or `jq`, or
`bwrap`)**

The workspace image is missing a driver dependency. The driver runs with a
controlled `PATH` of
`/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, so the tools
must be installed in those locations. Rebuild the image from the provided
`Dockerfile`.

**The declaration is reported as rejected with a shared-path policy reason**

The declared `shared_host_path` must be an existing directory inside the
parent agent's `directory`, must not be the parent's home root, and must not
overlap `~/.ssh` or the launcher's private state. The declared
`shared_child_path` must be absolute, lexically clean, and must not overlap
the child's reserved directories (`/bin`, `/dev`, `/etc`, `/home`,
`/opt/coder`, `/proc`, `/run`, `/tmp`). This template's
`/home/coder/project` and `/workspace/project` satisfy all of that.

## Validation notes

`terraform validate` against a released `coder/coder` provider fails with an
unsupported resource type for `coder_subagent_execution`. That is expected
until the provider branch is released; use the provider branch described in
the prerequisites. `terraform fmt -check` passes as-is.

`docker build .` succeeds and produces an image whose `bwrap`, `jq`, and
static `busybox` are all on the driver's controlled `PATH`, with
`/home/coder/project` owned by `coder`. Launching the sandbox end to end
additionally needs a Docker daemon that permits nested unprivileged user
namespaces; see the troubleshooting section above.
