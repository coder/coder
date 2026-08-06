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
- a child-side workload, `scripts/probe.sh`, that probes the sandbox
  boundary, writes its report into the shared directory, starts BusyBox
  `httpd` in daemon mode, and then exits so the child agent can reach ready and
  expose the owner-shared `Sandbox probe report` app.

Everything else the child sees is created by the driver: an empty private
root, a minimal BusyBox `/bin`, generated account files, a private home,
temporary, and runtime directory, and the Coder binary read-only.

## Build and version compatibility

Nothing in this section is optional. Neither half of the feature is released,
so a released Coder and a released provider both fail.

### Coder server and agent

The deployment must be built from the same commit as this template, that is,
from the commit that contains `agent/subagentexec` (the parent-side launcher),
the `coder_subagent_execution` persistence and pre-created isolated child in
`coderd`, and this directory. Read that commit and the version string it
produces with:

```console
git rev-parse HEAD
./scripts/version.sh
```

The parent agent hands its own executable to the driver, and the driver binds
it into a sandbox that has no host library directories. So the agent binary
that the deployment serves has to be a *static* build of that same commit.
Build it with exactly:

```console
make build/coder-slim_linux_amd64
```

That target writes two files:

- `build/coder-slim_linux_amd64`, the make output;
- `site/out/bin/coder-linux-amd64`, the copy `coderd` serves to agents.

`coderd` reads the `site/out/bin` artifacts, so a running deployment keeps
serving whatever it started with. **Restart `coderd` after the build and
before creating a workspace**, otherwise the workspace agent downloads the
previous binary and the sandbox runs the wrong Coder. `scripts/preflight.sh`
checks that both files exist, are statically linked, and report the current
`HEAD` in their version string.

### Provider

`coder_subagent_execution` exists in `coder/terraform-provider-coder` at
commit `db27531e1cbb46f4365f0d352d54deaa82405694`. That commit is
**unreleased**: no published provider version contains the resource, so
leaving `source = "coder/coder"` unpinned does *not* select it. Terraform
resolves an unpinned source to the newest release, which does not have the
resource, and the plan fails with an unsupported resource type.

Build the provider from that commit into a directory of its own. A
`dev_overrides` entry names a *directory*, and Terraform looks inside it for
an executable called `terraform-provider-coder`:

```console
git clone https://github.com/coder/terraform-provider-coder
cd terraform-provider-coder
git checkout db27531e1cbb46f4365f0d352d54deaa82405694
sudo mkdir -p /opt/terraform-provider-coder
go build -o /opt/terraform-provider-coder/terraform-provider-coder .
```

Then point Terraform at that directory with a CLI configuration file, for
example `/etc/coder-dev.tfrc`:

```hcl
provider_installation {
  dev_overrides {
    "coder/coder" = "/opt/terraform-provider-coder"
  }

  # Everything else, including kreuzwerker/docker, is still installed from
  # the registry. An empty direct block is required: without it, dev_overrides
  # is the only installation method and the other providers cannot be found.
  direct {}
}
```

The override has to be visible to **the Terraform process the Coder
provisioner runs**, not to your shell. That means:

- the directory named in `dev_overrides` must contain the built
  `terraform-provider-coder` binary and must be readable by the provisioner;
- the CLI configuration file must be readable by the provisioner;
- `TF_CLI_CONFIG_FILE` must be set in the provisioner's environment, for
  example `TF_CLI_CONFIG_FILE=/etc/coder-dev.tfrc`.

For built-in provisioners this is the `coderd` process itself, so set
`TF_CLI_CONFIG_FILE` where `coderd` is started. For a containerized
`coderd` or an external `provisionerd`, bind mount **both** paths into
**every** container that runs a provisioner and set the variable there, for
example:

```console
docker run \
  -v /opt/terraform-provider-coder:/opt/terraform-provider-coder:ro \
  -v /etc/coder-dev.tfrc:/etc/coder-dev.tfrc:ro \
  -e TF_CLI_CONFIG_FILE=/etc/coder-dev.tfrc \
  ...
```

A provisioner that does not see the override fails the build with an
unsupported resource type for `coder_subagent_execution`. Terraform also
prints a warning on every run that a development override is in effect; that
warning is expected here.

## Host prerequisites

- **A plain rootful Linux Docker daemon.** bubblewrap has to create a user
  namespace inside the workspace container and map identities into it, which
  requires a range it can subdivide. A daemon started with `userns-remap`, a
  rootless daemon, or a daemon that is itself nested inside another user
  namespace gives the container a narrowed range, and the driver then fails
  with `bwrap: setting up uid map: Permission denied`. Confirm with
  `docker info` (no `name=userns` and no `name=rootless` security option) and
  with `head -n 1 /proc/self/uid_map` inside the workspace container, which
  must read `0 0 4294967295`.
- **Unprivileged user namespaces permitted by the host kernel.**
  `/proc/sys/user/max_user_namespaces` must be greater than zero, and
  `kernel.unprivileged_userns_clone` must be `1` where that knob exists. The
  container adds no capabilities.
- **`seccomp=unconfined`, `apparmor=unconfined`, and
  `systempaths=unconfined` on the workspace container.** Docker's default
  seccomp profile blocks the `mount` and `pivot_root` calls bubblewrap issues
  inside its own user namespace, and on AppArmor hosts (Ubuntu 23.10 and later
  set `kernel.apparmor_restrict_unprivileged_userns=1`) the `docker-default`
  profile denies unprivileged user namespace creation. Docker also masks and
  makes parts of `/proc` read-only by default, which prevents bubblewrap from
  mounting the child's fresh procfs unless system-path confinement is disabled.
  All three settings are declared in `main.tf`. They relax the parent
  container's confinement only; they are not part of the boundary the driver
  builds around the child. A daemon whose policy forbids these options cannot
  run this template.
- **A statically linked Coder binary**, as described under
  [Build and version compatibility](#build-and-version-compatibility). A
  dynamically linked, CGO-enabled, or BoringCrypto/FIPS build cannot execute
  inside the minimal root.
- **A statically linked BusyBox in the workspace image.** It becomes the
  child's entire userland, and no host library directory is bound into the
  sandbox. The provided `Dockerfile` installs `busybox-static` and fails the
  image build if the BusyBox the driver would resolve turns out to be
  dynamically linked. It also installs the driver's other host tools,
  `bubblewrap`, `jq`, and `ca-certificates`.
- **A fresh project volume with the right ownership.** The shared directory
  must exist and be owned by `coder` before the parent agent handles its
  manifest, because the shared-path policy resolves it during reconciliation,
  earlier than any `coder_script`. The `Dockerfile` creates
  `/home/coder/project` owned by `coder`, and Docker seeds a new empty named
  volume from the image directory it covers, so a first-time workspace is
  correct. A volume left over from an earlier image does not get reseeded: if
  the shared directory turns out to be root-owned, the probes cannot write
  their report. Delete the workspace and then its volume,
  `docker volume rm coder-<workspace-id>-project`, and create the workspace
  again. The volume declares `lifecycle { ignore_changes = all }`, so
  Terraform will not remove it for you.
- **`host.docker.internal` reachability** for a local deployment. The
  template already adds the `host-gateway` host entry and rewrites a loopback
  access URL in the parent's init script.
- No Docker socket and no parent home directory are mounted anywhere in this
  template. Only `/home/coder/project` is a persistent volume; the parent's
  home directory is container-local.

## Preflight

Before pushing the template, check this checkout and this Docker host:

```console
./scripts/preflight.sh
```

It is read-only: it starts nothing, writes nothing, and prints no environment
variable or credential. Every line is `PASS`, `WARN`, `FAIL`, or `NOTE`, and
it ends with a `SUMMARY` line. It exits nonzero when the agent artifact is
missing, stale, or dynamically linked, or when Docker is unusable, and it
warns rather than fails for kernel and daemon settings it cannot observe from
where it runs.

It cannot prove that bubblewrap will build a nested sandbox: it reads
settings, it never creates a namespace. Only the gated integration test and a
real workspace can tell you that. Both are in
[Validation](#validation).

## Demo steps

1. Push the template from this directory:

   ```console
   coder templates push linux-bwrap-subagent -d .
   ```

   The `docker_image` resource builds the reference image from the
   `Dockerfile` in this directory.

2. Create a workspace from the template and wait for the parent agent to
   connect.

3. In the dashboard, open the workspace and confirm, on the parent agent row:
   - the parent agent `main` is connected;
   - a nested `child agent` card appears inside the parent's row, showing the
     name `sandbox`, its connection status, and an `Isolated execution`
     badge;
   - the `Sandbox Driver` metadata item on the parent reads `running`;
   - the `Sandbox Probes` metadata item on the parent shows the probe
     summary, for example `25 passed, 0 failed, 25 total`.

   The child card renders the child's name, status, badge, and apps, and
   nothing else. There is no terminal button and no launcher log view for the
   child in the dashboard.

4. Once the child reports connected, the owner-shared `Sandbox probe report`
   app appears on the child card. Open it. It is the BusyBox HTTP server the
   child runs on port 3000, serving the report the child wrote from inside
   the sandbox.

5. From the parent workspace terminal, confirm that the one shared directory
   really is shared in both directions:

   ```console
   # The parent-side collaboration marker, written before the parent agent
   # started, is what the child's "parent shared marker is visible" probe
   # looks for.
   cat /home/coder/project/parent-shared-marker.txt

   # The child wrote both of these into the same directory from inside the
   # sandbox.
   cat /home/coder/project/probe-results.txt
   ls -l /home/coder/project/index.html

   # A new parent-side file shows up in the sandbox too, because the mount is
   # read-write in both directions.
   echo "hello from the human" > /home/coder/project/from-parent.txt
   ```

## The probe report

`scripts/probe.sh` runs as the child's `coder_script`, inside the sandbox, and
writes two files into `/workspace/project`, which is `/home/coder/project` on
the parent side:

- `probe-results.txt`, the plain-text report;
- `index.html`, the same report HTML-escaped, which is what the
  `Sandbox probe report` app serves.

The report starts with a header (generation timestamp, the child's user and
uid), then one line per check:

- `PASS: <description>` for a boundary that holds;
- `FAIL: <description>` for a boundary that does not;
- `NOTE: ...` for context that is not a result and never changes the counts.

It ends with `SUMMARY: <passed> passed, <failed> failed, <total> total`. A
healthy sandbox produces `SUMMARY: 25 passed, 0 failed, 25 total`. The parent
agent's `Sandbox Probes` metadata item is that summary line, so a regression
is visible from the workspace page without opening the app.

The 25 checks cover five boundaries:

- **the shared directory**, which must exist, be writable, and contain
  `parent-shared-marker.txt`. That marker is the collaboration signal: the
  parent side writes it before the parent agent starts, and the child seeing
  it is what proves the two sides share one real directory rather than two
  copies;
- **parent-only state**, which must be absent: a parent dotfile, a parent
  `~/.ssh` key file, and a parent private directory. `scripts/parent-fixtures.sh`
  creates all three in the workspace container as harmless plain-text markers,
  so the absence probes have something real to look for;
- **the host filesystem**, where `/usr`, `/lib`, `/root`, `/var`, and both
  Docker socket paths must be absent;
- **the sandbox's own environment**, where `HOME`, `TMPDIR`, and
  `XDG_RUNTIME_DIR` must be the exact private paths the driver created and
  must be writable, `/proc/self/status` must be readable, `/dev/null` and
  `/dev/urandom` must be character devices, and `/dev/kmsg` must be absent;
- **the child's credential**, where the token file must be readable and not
  writable, and `CODER_AGENT_TOKEN` must be unset in the environment.

The report includes one `NOTE` that carries a real caveat: network isolation
is untested, because the driver deliberately shares the host network
namespace, so no probe makes any claim about it.

The report never contains the child's token. The token probes read the file's
mode bits only, and the environment probe uses `${CODER_AGENT_TOKEN+set}`,
which reports whether the variable exists without expanding its value.
`scripts/probe_harness_test.sh` asserts that a seeded token value never
reaches either output file.

## Support claim

Only the checked-in `drivers/bwrap.sh` in this directory is the vetted
reference driver for the PoC. The sandbox properties described above are
claimed for exactly that script, byte for byte, invoked by the Coder-owned
manager, on a host that meets the prerequisites above. Editing it voids the
claim.

Custom drivers speak the same protocol (`run` and `cleanup`, protocol
version 1, one JSON input document), and they are **trusted, not certified**.
Coder cannot prove sandbox properties for an arbitrary script: a custom
driver is as trusted as the template that ships it.

## Limitations

- **Network is shared, and there is no egress isolation.** The driver
  deliberately does not unshare the network namespace, because the child agent
  must reach the deployment. The child can reach anything the workspace
  container can reach. The probes make no claim about the network, and say so
  in the report.
- **No Agent Identity.** The child has a workspace-agent token and nothing
  else. It has no agent principal, no agent-owned credentials, and no Git or
  external-auth material. Human credentials are simply absent, which is a
  "no principal" state rather than an identity model. The suppression is
  temporary: `GetManifest` omits the workspace owner's secrets and Git auth
  configuration for any execution-isolated agent until Agent Identity gives
  the child its own principal and its own credentials.
- **`RUNNING` means the driver process started**, not that the child agent is
  ready, not that the child connected, and not that the sandbox is healthy.
- **`startup_timeout` and `restart_policy` are declared but not enforced.**
  Both values are persisted and delivered to the parent agent, and the manager
  does not yet implement readiness timeouts or automatic restart.
  `restart_policy = "never"` therefore describes the behavior you actually get
  today; `on-failure` is the schema default and would not change current
  behavior either. A driver that exits is not restarted until the next
  reconciliation trigger.
- **The shared project directory is fully shared, by design.** Anything the
  human owner places inside `/home/coder/project`, including keys, tokens,
  sockets, or checkouts, is readable and writable by the child. The contract
  is about paths, not contents: keeping something away from the child means
  keeping it out of the project directory.
- **Minimal BusyBox userland.** The child has one static BusyBox and the
  applets the driver exposes. There is no package manager, no compiler
  toolchain, no `bash`, and no container runtime. Child-side scripts must be
  POSIX shell using those applets.
- **Only the project directory persists.** The parent's home directory and
  everything private to the sandbox are container-local and are recreated on
  rebuild.
- **A dynamic, CGO-enabled, or BoringCrypto Coder binary is unsupported** by
  the minimal root, as described in the prerequisites. This is a hard
  incompatibility, not a degraded mode: the child agent never starts.

## Troubleshooting

**`bwrap: setting up uid map: Permission denied`**

The user namespace was created and the kernel then refused the identity
mapping, which means the workspace container is already inside a user
namespace whose range it cannot subdivide. Run `head -n 1 /proc/self/uid_map`
inside the workspace: anything other than `0 0 4294967295` means the
container is remapped. Move to a rootful Docker daemon without
`userns-remap`, as described in the prerequisites.

**`No permissions to create new namespace` or `Creating new namespace
failed`**

The host or container is denying unprivileged user namespace creation. Check,
in order, on the Docker host:

```console
head -n 1 /proc/sys/user/max_user_namespaces           # must be > 0
sysctl kernel.unprivileged_userns_clone                # if present, must be 1
sysctl kernel.apparmor_restrict_unprivileged_userns    # if present, 1 needs apparmor=unconfined
```

**`bwrap: pivot_root: Operation not permitted`, `Can't mount proc`, or a
failure during mount setup**

One of Docker's default security restrictions is still in effect. Confirm the
container was recreated with all three `security_opts` this template declares:
`seccomp=unconfined`, `apparmor=unconfined`, and
`systempaths=unconfined`. Updating the template does not modify an existing
container in place; update or recreate the workspace so Terraform replaces it.

**The child agent never connects, and the driver fails with an exec format or
loader error**

The Coder binary the manager passed is not statically linked, or `coderd` is
still serving an older `site/out/bin/coder-linux-amd64`. Rerun
`make build/coder-slim_linux_amd64`, restart `coderd`, and delete and recreate
the workspace. `./scripts/preflight.sh` checks both artifacts.

**`bwrap driver: required tool not found on PATH: busybox` (or `jq`, or
`bwrap`)**

The workspace image is missing a driver dependency. The driver runs with a
controlled `PATH` of
`/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, so the tools
must be installed in those locations. Rebuild the image from the provided
`Dockerfile`.

**The probe report is missing, or `probe: shared project directory does not
exist`**

The child could not write into the shared directory. The usual cause is a
pre-existing project volume whose ownership predates the image's `chown`; see
the fresh-volume item in the prerequisites.

**The declaration is reported as rejected with a shared-path policy reason**

The declared `shared_host_path` must be an existing directory inside the
parent agent's `directory`, must not be the parent's home root, and must not
overlap `~/.ssh` or the launcher's private state. The declared
`shared_child_path` must be absolute, lexically clean, and must not overlap
the child's reserved directories (`/bin`, `/dev`, `/etc`, `/home`,
`/opt/coder`, `/proc`, `/run`, `/tmp`). This template's
`/home/coder/project` and `/workspace/project` satisfy all of that.

## Validation

Run these from this directory, in this order.

1. **Preflight.** Read-only, and the cheapest way to find a stale artifact or
   an unusable daemon:

   ```console
   ./scripts/preflight.sh
   ```

   Expect `FAIL` lines for both agent artifacts on a checkout that has not
   been built yet, and a zero-`FAIL` `SUMMARY` after
   `make build/coder-slim_linux_amd64`. `WARN` lines are not failures: they
   mark settings this process cannot decide from outside the workspace
   container.

2. **The probe harness**, which runs `scripts/probe.sh` against synthetic
   directory trees, so the probe logic is exercised without a sandbox:

   ```console
   ./scripts/probe_harness_test.sh                 # with /bin/sh
   ./scripts/probe_harness_test.sh /bin/busybox sh # with BusyBox, as in the sandbox
   ```

   It refuses to run as root, because root ignores the mode bits the
   read-only probes depend on.

3. **The gated bubblewrap integration test**, from the repository root. This
   is the only check short of a real workspace that actually builds a sandbox
   with the checked-in driver and inspects it from the inside:

   ```console
   go test ./agent/subagentexec -run TestBwrapDriverSandboxIsolation -v
   ```

   It skips, with an explicit reason, on a machine that cannot host a
   sandbox, so read its output: a skip is not a pass. Once its own capability
   probe succeeds, every property it checks is a hard requirement.

4. **Terraform formatting.**

   ```console
   terraform fmt -check
   ```

   This passes as-is. `terraform validate` and `terraform plan` are a
   different matter: against a released `coder/coder` provider they fail with
   an unsupported resource type for `coder_subagent_execution`, and they only
   succeed with the `dev_overrides` configuration described under
   [Provider](#provider). Expect Terraform to warn about the development
   override on every run.

5. **The image.** `docker build .` succeeds and produces an image whose
   `bwrap`, `jq`, and static `busybox` are all on the driver's controlled
   `PATH`, with `/home/coder/project` owned by `coder`.

6. **The end-to-end demo.** Nothing above proves that bubblewrap can build a
   nested sandbox inside a workspace container on your Docker host. The
   complete check is creating a workspace and reading the report: the child
   card shows `Isolated execution` and connects, the parent's `Sandbox Probes`
   metadata reads `25 passed, 0 failed, 25 total`, and the
   `Sandbox probe report` app serves a report whose only non-`PASS` line is
   the `NOTE` about network isolation.
