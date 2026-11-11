# Results

Date: 2026-11-11. Cluster: single-node k3s v1.36.5 with Cilium, in a QEMU VM
under software emulation. coderd: built from this branch, one replica,
outside the cluster. Runtime: Docker 27 inside a privileged workspace pod.

## Outcome

Every check in [validate.py](validate.py) passed: 36 of 36, in 71 seconds
including a workspace stop and start.

Go tests on the branch pass: `agent/agentworkload` (the Docker runtime's
container and channel), and `coderd` `TestWorkloads` and
`TestWorkloadsDisabled` (the whole flow against a test coderd and a real
workspace daemon, with a fake runtime whose sandboxes the test plays).

## Speed

`coder run dev -- true`, from the command line to the exit code, five runs:
median 1.43 s, range 1.35 s to 1.53 s. The first run in a new workspace
took 2.9 s.

The earlier design on this branch's parent ran each workload in its own pod
with its own workspace daemon. On the same cluster, on the same day, the
same command took 9.1 s, 10.1 s and 13.1 s there. It also needed an operator
in the cluster, and a new pod can make a cluster add a node. Here nothing is
scheduled: no pod, no workspace build, no new agent.

These numbers come from a VM without hardware virtualization, so they are
slower than a real node would be.

## What the checks show

**A. The daemon and the human (3 checks).** The workspace daemon has its own
identity, resting on its agent token. The human's CLI shows the user and no
workload identity.

**B. Running workloads (5 checks).** `coder run dev -- coder identity`
prints an identity of the form `<workspace UUID>/workload/<identity UUID>`,
sponsored by the caller, with no scopes. Output is streamed and the exit
code returned. Two detached workloads run side by side in separate
containers with separate identities. No build ran and no pod or agent was
added.

**C. Isolation and masquerade attempts (11 checks).**

- A workload that claims to be another in headers, cookie, bearer token and
  query is still itself.
- Its sandbox holds no credential: only the path of its channel.
- It runs as an unprivileged user without capabilities, sees its channel
  read-only, and sees nothing of the workspace, of Docker or of other
  workloads: no files, no processes.
- It cannot reach another workload's sandbox or the Docker API over the
  network.
- It cannot use the daemon's own API, although its requests reach coderd
  with the daemon's token.
- Going around the daemon, straight to coderd, with another workload's
  identity in a header gets it nothing.
- From outside, no credential, a random agent token, another workspace's
  daemon, or the right daemon naming an identity that does not exist are all
  refused.
- A user who sends the identity header stays that user.
- By design, the daemon's own token can act as the workloads it runs.

**D. Permissions (7 checks).** Without scopes a workload sees no
workspaces, templates or organizations and cannot read its own workspace,
even while a workload with read scopes runs beside it. With `template:read`
it lists its sponsor's templates. With `workspace:read` and `template:read`
it lists exactly the workspaces its sponsor can see, and `coder list` works
inside it without a login. With `workspace:update` it can change the
workspace but still cannot run or remove workloads. An administrator who
does not own a workspace cannot run a workload in it.

**E. Revocation and lifecycle (5 checks).** Removing a workload revokes its
identity at once, even for its own daemon, and stops its sandbox. An
identity is revoked when its command exits, or when its sandbox is killed
behind Coder's back. Stopping the workspace ends every workload. Starting it
again brings back the daemon and no workloads, and the new daemon cannot
vouch for an old identity.

**F. Footprint (2 checks).** The namespace holds only workspaces: no
operator, no extra service account, role or secret, and nothing bound to a
cluster-wide role.

## Limits

- **The workspace daemon is trusted to vouch.** Root in the workspace, and
  so its owner, can act as any workload in it (check C11). Nothing outside
  the workspace attests a workload, as Kubernetes did for pods.
- **Isolation is the runtime's.** Docker containers share the workspace's
  kernel. A workload that escapes its container is root in the workspace and
  holds the daemon's token. Only the Docker runtime exists; microVM and
  namespace runtimes are not built.
- **The workspace pod is privileged**, because it runs Docker. The human has
  root in it.
- **Workloads share the workspace's resources.** There are no CPU or memory
  limits per workload yet, and the workspace must be sized for them.
- **No shared files.** A sandbox gets its image's filesystem and nothing of
  the workspace. A workload cannot see the human's checkout.
- **Network policy is the operator's.** A sandbox has ordinary outbound
  network access. It can reach coderd directly, where it is anonymous.
- **No stdin or terminal.** `coder run` streams output only.
- **Workloads do not survive a daemon restart.** The daemon removes
  leftover sandboxes when it starts, because their channels died with it.
- **Output is not kept.** It can be read while the command runs and for two
  minutes after.
- **Scopes are coarse.** They are API token scopes, so they limit the kind
  of action and not the object. They cannot be changed after `coder run`.
- **No Git credentials at all.** There is no scope that grants a workload
  its sponsor's Git SSH key or external auth tokens.
- **Nothing is audited as the workload.** On the API a workload's requests
  are recorded as its sponsor's.
- **An exited workload's identity is revoked lazily** when nobody is
  following its output: on the next listing. It cannot be used in between,
  because its sandbox is gone.
