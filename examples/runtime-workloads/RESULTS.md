# Results

Date: 2026-11-11. Cluster: single-node k3s v1.36.5 with Cilium, in a QEMU VM
under software emulation. coderd: built from this branch, one replica,
outside the cluster. Runtime: Docker 27 in rootless mode, inside a workspace
pod that runs in a user namespace and has no privileged container.

## Outcome

Every check in [validate.py](validate.py) passed: 55 of 55, in 104 seconds
including a workspace stop and start. Five of the checks read the audit log,
which needs the enterprise build and a license. Without them the script runs
the other 50.

Go tests on the branch pass: `agent/agentworkload` (the Docker runtime's
container and channel), and `coderd` `TestWorkloads` and
`TestWorkloadsDisabled` (the whole flow against a test coderd and a real
workspace daemon, with a fake runtime whose sandboxes the test plays).
`TestWorkloads` also covers a suspended sponsor, and asserts the audit
entries for deployments without a license.

[EVALUATION.md](EVALUATION.md) is the review of what this means and what is
missing.

## Speed

`coder run dev -- true`, from the command line to the exit code, five runs:
median 1.58 s, range 1.45 s to 2.17 s. The medians of the four runs before
were 1.40 s, 1.55 s, 1.48 s and 1.51 s. The first run in a new workspace
took 3.0 s.

Hardening cost little. The previous version of this branch ran Docker in a
privileged pod and measured a median of 1.43 s.

The earlier design on this branch's parent ran each workload in its own pod
with its own workspace daemon. On the same cluster, on the same day, the
same command took 9.1 s, 10.1 s and 13.1 s there. It also needed an operator
in the cluster, and a new pod can make a cluster add a node. Here nothing is
scheduled: no pod, no workspace build, no new agent.

These numbers come from a VM without hardware virtualization, so they are
slower than a real node would be.

## What the checks show

**A. The daemons and the human (3 checks).** The human's daemon has its own
identity, resting on its agent token. The human's CLI shows the user and no
workload identity.

**B. Running workloads (6 checks).** `coder run dev -- coder identity`
prints an identity of the form `<workspace UUID>/workload/<identity UUID>`,
sponsored by the caller, with no scopes. coderd picks the daemon that runs
workloads by itself. Output is streamed and the exit code returned. Two
detached workloads run side by side in separate containers with separate
identities. No build ran and no pod or agent was added. `coder run -it`
gives a terminal in a sandbox: what is typed runs as the workload, and the
shell's exit code comes back.

**C. Isolation and masquerade attempts (12 checks).**

- A workload that claims to be another in headers, cookie, bearer token and
  query is still itself.
- Its sandbox holds no credential: only the address of its channel.
- It runs as an unprivileged user without capabilities, sees its channel
  read-only, and sees nothing of the workspace, of Docker or of other
  workloads: no files, no processes.
- It cannot reach another workload's sandbox or the Docker API over the
  network, even with outbound network access.
- It cannot use the daemon's own API, although its requests reach coderd
  with the daemon's token.
- Going around the daemon, straight to coderd, with another workload's
  identity in a header gets it nothing.
- From outside, no credential, a random agent token, another workspace's
  daemon, or the right daemon naming an identity that does not exist are all
  refused.
- A user who sends the identity header stays that user.
- An ordinary HTTP client in the sandbox reaches Coder as the workload,
  through `$CODER_URL`, without a token.
- By design, the token of the daemon that runs workloads can act as them.

**D. Permissions (7 checks).** Without scopes a workload sees no
workspaces, templates or organizations and cannot read its own workspace,
even while a workload with read scopes runs beside it. With `template:read`
it lists its sponsor's templates. With `workspace:read` and `template:read`
it lists exactly the workspaces its sponsor can see, and `coder list` works
inside it without a login. With `workspace:update` it can change the
workspace but still cannot run or remove workloads. An administrator who
does not own a workspace cannot run a workload in it.

**E. The template's policy (6 checks).**

- By default a sandbox has one network interface, loopback. It cannot reach
  coderd directly and still reaches Coder as itself through its channel.
- Outbound network is given only to workloads that ask, and what each
  workload got is on its record.
- An image the template does not allow is refused, and the refused attempt
  is on record as `failed`.
- `--scope coder:all` is refused.
- A workload sees nothing of the workspace by default. A shared directory is
  readable and not writable. One shared with `:rw` is writable, and the
  human sees what the workload wrote.
- Sharing is refused for the human's home directory, for a path that leaves
  the allowed directory through `..` or through a symbolic link, for the
  directory that holds the channels, and for Docker's socket.

**F. Where workloads run (4 checks).**

- The pod has `hostUsers: false` and no privileged container. User 0 in the
  pod is user 2547056640 on the node.
- The human's container has no added capabilities, no Docker and no view of
  the channels.
- The daemon that runs workloads is refused the owner's Git SSH key (403),
  which the human's daemon gets (200).
- The human's daemon cannot vouch for a workload (401).

**G. Revocation, lifecycle and the record (7 checks).**

- Removing a workload ends its identity at once, even for its own daemon,
  stops its sandbox, and is recorded as `removed`.
- A detached workload that nobody watches exits with code 5. Its record
  says `exited (5)` and its identity is refused from then on.
- Its output and exit code can still be read afterwards.
- A sandbox killed behind Coder's back ends the workload and its identity.
  It is recorded as `exited (137)`.
- Stopping the workspace ends every workload, recorded as `stopped`.
- After the stop, the workloads of the run are still on record, each with
  how it ended.
- Starting the workspace again brings back the daemons and no workloads.
  The new daemon cannot vouch for an old identity.

**H. Footprint (2 checks).** The namespace holds only workspaces: no
operator, no extra service account, role or secret, and nothing bound to a
cluster-wide role.

**I. The audit log (5 checks).**

- Each of the 29 workloads of the run that are over has a start entry and
  an end entry.
- A start entry names the user, the workload, the time, the workspace and
  its daemon, the command, the scopes, the image and the network, and links
  to the workspace.
- An end entry says how it ended: `exited` with code 5, `removed` by a
  user, `stopped` with the workspace, or `failed` for a refused image,
  whose start entry carries the status 502.
- A terminal attached to a workload is a `connect` entry under the person
  who attached.
- A workspace change made by a workload is audited under its sponsor and
  marked with the workload's identity.

The entries were also looked at on the audit page, where they read
"admin started workload survivor" and link to the workspace.

## What was checked another way

- **The sponsor in person.** `TestWorkloads` makes the same workspace
  change as a workload and as its sponsor, and asserts that only the first
  is marked with a workload identity.
- **A suspended sponsor.** The same test suspends the sponsor, sees the
  workload refused, reactivates the sponsor, and sees it work again.
- **Disk snapshots** are not built. `docker commit` of an exited sandbox and
  a new sandbox from the result were tried by hand in the pod and worked.
  See [DESIGN.md](DESIGN.md#disk-snapshots).

## What was found on the way

- **Docker cannot use cgroups in a user-namespaced pod.** Kubernetes does
  not delegate them. Docker's rootless mode runs without, which is why
  workloads have no CPU, memory or process limits of their own.
- **The typed Kubernetes resources of the Terraform provider cannot express
  the pod.** `hostUsers`, `procMount` and the AppArmor profile need a raw
  manifest, and the manifest resource fails on an empty environment value.
- **A terminal lost its input.** The `coder` program that starts the command
  in the sandbox asked the terminal a question at startup and swallowed
  what was typed in the meantime. It now sees a dumb terminal and hands the
  real terminal type to the command.
- **Records outlive runs, and names repeat.** The validation script first
  mistook the record of an earlier run's workload for the one it was
  waiting for. Anything that looks up a finished workload by name must
  expect several.

## Limits

[EVALUATION.md](EVALUATION.md) lists them with their consequences. In
short:

- **Agents have no mediated model access yet.** AI Gateway does not accept
  workload identities. Designed in [DESIGN.md](DESIGN.md#agents).
- **No agent harness was run.** The terminal, shared directories, images
  and identity were tested with shells and scripts.
- **The daemon is trusted to vouch**, and the workspace owner can enter its
  container.
- **Isolation is a container on a shared kernel.** Only the Docker runtime
  exists. The sandbox host needs `CAP_SYS_ADMIN` in its user namespace and
  runs without seccomp and AppArmor profiles.
- **Egress is all or nothing**, and not logged.
- **No resource limits per workload.**
- **Workloads do not survive a daemon restart or an autostop.**
- **Scopes are coarse**, cannot be capped centrally, and cannot be changed
  after `coder run`.
- **No Git credentials and no secrets** for a workload.
- **Reads are not audited**, only logged at debug level.
- **Agent Sandbox was not tried** as a runtime. See
  [DESIGN.md](DESIGN.md#agent-sandbox).
- **Records are never purged**, and only the last 16 KB of output is kept.
- **One cluster, one coderd replica.** Kubernetes user namespaces are
  required for the example template.
