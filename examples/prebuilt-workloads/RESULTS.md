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
| `coder run --template prebuilt -- true`, five runs, pool full | median 2.1 s, 2.0 s to 2.2 s |
| The same in the four runs before                              | medians of 2.0 s to 2.3 s    |
| Five at once on a pool of three: the three that claimed       | 2.4 s to 4.9 s               |
| The two that got a workspace built for them                   | 9.1 s and 10.4 s             |
| From a claim until the pool was full again, one try           | 9 s                          |
| The sponsor entering the workspace with `coder ssh`           | 0.8 s                        |

Nearly all of the two seconds pass before the workload has started: the
claim's Terraform run, which changes nothing, and the daemon starting over
for its new owner. The output and the exit code take 10 ms more.

## What was shown

- **The identity owns the workspace.** The owner is a service account that
  cannot sign in, not the sponsor. The pod was running before the workload
  was asked for: the workspace was claimed, not built.
- **The cluster vouches for it.** The workload's identity names the pod's
  UID, and the sponsor.
- **The workload has a token of its own.** It is the identity account's
  only token, it names the workload, and it carries the workload's scopes.
  With it the workload can ask who it is and read its own workspace, and
  not another workload's. It is refused once the workload is removed.
- **Nothing is inherited.**
  - Without scopes the workload sees no template, no workspace and no user.
  - With `template:read` it sees templates and nothing else.
  - Allowed to read workspaces, it sees its own and nobody else's.
  - The Git key its daemon is given is the identity's own.
- **Nobody is somebody else.**
  - A forged token is refused.
  - A pod that waits in the pool is the `prebuilds` account's. What it
    holds is no user's credential and no workload's.
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
claimed. It also covers a template that is open to the sponsor and to
nobody else: the workload's workspace is made and deleted all the same.

## Not shown

- **A template that is restricted to groups, on the cluster.** Only the
  test covers it.
- **The sponsor's terminal session in the audit log.** It was not looked
  for.
- **A burst much larger than the pool,** or many sponsors at once.
- **A terminal with `-it`** is not in the script. One try by hand worked:
  the command had a terminal and its exit code came back.
- **One cluster, one coderd replica, one day.**
