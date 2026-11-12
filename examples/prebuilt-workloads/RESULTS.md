# Results

What [validate.py](validate.py) showed on 12 November 2026, on one small
and slow cluster: a single k3s node in QEMU software emulation, with coderd
on the host. The template kept three workspaces ready, and coderd looked
every 15 seconds whether one needed replacing.

All 29 checks passed, in four minutes. The sponsor in every check is an
ordinary member, not an administrator.

## Speed

| What                                                          | Time                         |
|---------------------------------------------------------------|------------------------------|
| `coder run --template prebuilt -- true`, five runs, pool full | median 2.0 s, 1.9 s to 2.4 s |
| The same in the two runs before                               | median 2.3 s both times      |
| Five at once on a pool of three: the three that claimed       | 2.3 s to 2.9 s               |
| The two that got a workspace built for them                   | 13.1 s and 17.5 s            |
| From a claim until the pool was full again, one try           | 9 s                          |
| The sponsor entering the workspace with `coder ssh`           | 0.8 s                        |

Of the two seconds, 1.85 s pass until the workload has started, which is
the claim's Terraform run and the daemon starting over for its new owner.
The output and the exit code take 0.3 s more.

## What was shown

- **The identity owns the workspace.** The owner is a service account that
  cannot sign in, not the sponsor. The pod was running before the workload
  was asked for: the workspace was claimed, not built.
- **The cluster vouches for it.** The workload's identity names the pod's
  UID, and the sponsor.
- **Nothing is inherited.**
  - Without scopes the workload sees no template, no workspace and no user.
  - With `template:read` it sees templates and nothing else.
  - Allowed to read workspaces, it sees its own and nobody else's.
  - The Git key its daemon is given is the identity's own.
  - The identity has no API token.
- **Nobody is somebody else.**
  - A forged token is refused.
  - A pod that waits in the pool is the `prebuilds` account's and cannot
    name a workload.
  - The session a pod had while it waited is refused once it is claimed.
  - One workload cannot act as another.
  - A workload cannot sponsor a workload.
- **The sponsor shares, and does not own.** The sponsor can read, enter and
  end it. The workspace is shared with the sponsor only, with the `use`
  role. The sponsor has no way into a workspace that is not shared.
- **Nothing is left behind.** When the command is over, the workspace, the
  pod and the identity's account are gone. The record stays, with the exit
  code, and the audit log has the workload's start and end and the
  workspace's build and removal.
- **What was there before still works.** A person claims a prebuilt
  workspace of the same template and owns it.

[coderd/workloadlaunch_test.go](../../coderd/workloadlaunch_test.go) covers
the same ground without a cluster, for a workspace that is built and not
claimed.

## Not shown

- **A template that is restricted to groups.** See the limits in
  [README.md](README.md).
- **The sponsor's terminal session in the audit log.** It was not looked
  for.
- **A burst much larger than the pool,** or many sponsors at once.
- **The example in [../sandbox-workloads](../sandbox-workloads)** was not
  checked again on this branch. One run of its template through the new
  request took 0.8 s and worked.
- **One cluster, one coderd replica, one day.**
