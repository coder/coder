# Results

Date: 2026-11-12. Cluster: single-node k3s v1.36.5 with Cilium, 4 CPUs, in a
QEMU VM under software emulation. Agent Sandbox v1.0.10, pool of 3 pods.
coderd: built from this branch, one replica, outside the cluster.

## Outcome

Every check in [validate.py](validate.py) passed: 37 of 37, in seven and a
half minutes.

A workload with a workspace and a pod of its own, vouched for by the
cluster, starts and returns in under a second. That is faster than a
sandbox inside an existing workspace's daemon, and it is isolated and
attested better. The price is a controller and idle pods in the cluster,
and that what a workload's pod is can no longer be chosen per run.

Go tests on the branch pass: the sandbox provisioner and the cluster client
against a stand-in for the Kubernetes API, the pod identity endpoint
including a daemon that waits to be claimed, the `pod` runtime, and the
earlier `TestWorkloads`.

## Speed

`coder run --template sandbox -- true`, from the command line to the exit
code, five runs with a full pool: median 0.93 s, range 0.64 s to 1.09 s.
The two validation runs before it measured medians of 0.78 s and 0.82 s.

| Design, on the same cluster                          | `coder run ... -- true` | What starts                       |
|------------------------------------------------------|-------------------------|-----------------------------------|
| A pod per workload, started on demand (first rounds) | 9.1 s to 13.1 s         | A pod, then a daemon in it        |
| A Docker sandbox inside a workspace's daemon         | about 1.5 s             | A container                       |
| A workspace of its own from a warm pool (this)       | 0.6 s to 1.1 s          | Nothing: a running pod is claimed |
| The same when the pool is empty                      | 30 s to 46 s            | A pod, then a daemon in it        |

Where the time goes in one run, as the CLI reports it with `--timing`:

| Step                                                               | Time             |
|--------------------------------------------------------------------|------------------|
| Find the template, create the workspace                            | 30 ms            |
| Build the workspace: claim a pod, register it                      | 310 ms to 950 ms |
| The pod's daemon learns who it is, fetches and starts the workload | 70 ms to 110 ms  |
| Return the output and the exit code of `true`                      | 15 ms            |

- **The claim itself takes 0.15 s to 0.25 s.** The rest of the build is
  Coder's own build pipeline, which was not tuned.
- **`coder create --template sandbox` takes 0.5 s to 0.9 s** for a
  workspace that is listed, can be entered and has a connected daemon a
  moment later. Importing the template takes 0.1 s.
- **A run in a workspace that exists already takes 0.2 s to 0.3 s.**
- **A terminal takes about 2 s longer.** `coder run -it` needs a connection
  from coderd to the new daemon, which the daemon sets up after it has been
  claimed. A command without a terminal does not wait for that connection:
  it had started and finished before the daemon was reachable.
- **Five workloads at once on a pool of three:** three returned after
  1.6 s to 2.7 s, the other two after about 40 s, when new pods had
  started. In the two runs before, the three took 1.3 s to 5.6 s and the
  two 30 s to 46 s.

These numbers come from a VM without hardware virtualization. Everything
that computes is slower there than on a real node, most of all a daemon's
start.

## What the checks show

**A. A workload is a workspace of its own (7 checks).** The identity names
the workload, its sponsor and the UID of its pod. The workspace is named
like the workload and was made for it. Output is streamed and the exit code
returned. `coder run -it` gives a terminal. Two workloads are two
workspaces in two pods. The sponsor can open a shell next to a running
workload with `coder ssh`.

**B. Who a pod is, is the cluster's word (8 checks).**

- The kubelet gave the pod a token that names the pod, is issued for Coder
  only, and lasts ten minutes. The pod's service account may touch nothing
  in the cluster, and no other token is mounted.
- coderd has the cluster review the token and tells the daemon which
  workspace it serves. No Coder token is in the pool's pod template.
- One workload's pod cannot be another: naming the other's identity, or
  using its scopes, is refused.
- A pod of the pool that nobody has claimed is nobody. Its token is genuine
  and gets it nothing.
- Refused: a token rewritten to name another pod, a token of the same
  service account that is bound to no pod, a token bound to the right pod
  but issued for the Kubernetes API, and no token.
- A pod that somebody else starts in the namespace with the same service
  account and audience is nobody.
- What a pod is given is its daemon's session. That is not a user's: no
  Git SSH key, no user API.

**C. Permissions (5 checks).** Without scopes a workload sees nothing. With
read scopes it sees what its sponsor can see, also with `wget` and no
token. Whatever its scopes, it cannot run workloads or list its sponsor's
others. `coder:all` is refused, and a refused run leaves no workspace.
Another user runs workloads of their own and cannot see, enter or remove
this sponsor's.

**D. The pod is the sandbox (4 checks).**

- The namespace enforces the Restricted Pod Security Standard and the pods
  pass it: nothing privileged, every capability dropped.
- Root in the pod is user 4157538304 on the node.
- The workload has limits of its own: 2 CPUs and 512 MiB, its pod's.
- The pod reaches coderd. It does not reach the Kubernetes API, the
  internet, or a port that another workload listens on.

**E. Lifecycle and the record (6 checks).**

- Removing a workload ends its identity, deletes its workspace and gives
  its pod back.
- When the command ends, outcome and output stay on record, and can be read
  after the workspace is gone.
- Deleting a workload's pod behind Coder's back ends the workload and its
  workspace. The daemon reports that it stops.
- A workspace kept with `--keep` stops and starts like any workspace.
  Stopping gives the pod back; starting claims another, and the daemon in
  it is vouched for anew.
- More workloads than the pool holds all run, each in its own pod.
- Afterwards the pool is full again, no claim is left, and all 20 workloads
  of the run are on record.

**F. The audit log (2 checks).** Every workload has a start and an end
entry, and the terminal session is recorded. Because a workload is a
workspace, its pod's coming and going is in the audit log as workspace
builds.

**G. Footprint (1 check).** The namespace holds the Agent Sandbox
controller and the pool. coderd's role allows creating, reading and
deleting claims, and reading the sandbox and pod a claim leads to. Outside
the namespace it may only ask for token reviews.

## The requirements

Against what was asked of workloads so far. "Before" is the design this
branch started from: a Docker sandbox inside a workspace's daemon.

| Requirement                                                   | Before                                | With a workspace of its own                                                                                                |
|---------------------------------------------------------------|---------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| Workloads are generic; other templates keep working           | Met                                   | Met. Sandbox templates are a new kind next to Terraform templates.                                                         |
| A workload has an identity with a sponsor and scopes          | Met                                   | Met, unchanged                                                                                                             |
| The default identity can do nothing; scopes are not inherited | Met                                   | Met, unchanged                                                                                                             |
| A workload cannot pass for another workload                   | Met; the daemon keeps them apart      | Met; the cluster keeps them apart                                                                                          |
| A workload cannot pass for the human                          | Met                                   | Met                                                                                                                        |
| A workload cannot pass for its workspace's daemon             | Met; it holds no credential           | Changed. The workload and the daemon of its pod are one principal.                                                         |
| The identity is non-repudiable                                | The daemon's word                     | The cluster's word, from the kubelet's token for the pod                                                                   |
| Only a workspace's owner runs workloads in it                 | Met                                   | Met. A workload's workspace belongs to its sponsor.                                                                        |
| No pod scheduling or node scale-up on the way                 | Met; nothing is scheduled             | Met while the pool has pods. The pool refills behind the run.                                                              |
| No Terraform for a run                                        | Met                                   | Met. No Terraform at all.                                                                                                  |
| Isolation, and a choice of it                                 | A container on the workspace's kernel | A pod, and the pool's runtime class where the cluster has one. Not tried with gVisor or Kata.                              |
| Resource limits per workload                                  | Not met                               | Met; the pod's                                                                                                             |
| Runs without privileges in the cluster                        | Needs added capabilities              | Met; Restricted Pod Security Standard                                                                                      |
| Nothing installed in the cluster                              | Met                                   | Not met. A controller, four resource types, a role for coderd, idle pods.                                                  |
| `coder run` like `docker run`: output, exit code, `-d`, `-it` | Met                                   | Met. Output and error output are still one stream.                                                                         |
| `coder identity`, `coder whoami`                              | Met                                   | Met                                                                                                                        |
| Image, network and shared files chosen per run                | Met, within the template's policy     | Not met. They are the pool's. A user chooses a template.                                                                   |
| The human's files can be shared with a workload               | Met, by directory                     | Not met. The workload's pod is not the human's.                                                                            |
| Workloads are listed, finished ones included                  | Met, per workspace                    | Met, across workspaces: `coder run --all` and a Workloads page                                                             |
| The audit log says what ran and when                          | Met                                   | Met, and workspace builds are audited too                                                                                  |
| A workload can be a workspace: shell, ports, editors          | Not met                               | Met. It is one.                                                                                                            |
| Agents with model access and no key                           | Not built                             | Not built. Nothing here changes it.                                                                                        |
| A disk that outlives the workload                             | Designed                              | Not built. Stop and start of a kept workspace is where it belongs: Agent Sandbox can keep volumes for a suspended sandbox. |
| Works outside Kubernetes                                      | Met, wherever there is Docker         | Not met                                                                                                                    |

## What was found on the way

- **A daemon in a warm pod must be told when it is claimed.** A workspace
  daemon that cannot authenticate retries with a pause that grows to ten
  seconds. The pod identity endpoint therefore holds the request of a pod
  that nobody has claimed, and answers when a build registers the pod.
- **coderd calling the daemon was the slow part.** A daemon that has just
  been claimed needs about two seconds on this cluster before coderd can
  reach it. With coderd starting the workload over that connection, a run
  took 3.3 s. Now the daemon asks coderd for its workloads, and a short
  command is over before the connection exists.
- **The daemon's own start competes with the workload.** With a limit of
  one CPU on the pod, the command took a second longer. The pool's limit
  is two.
- **Looking less often costs more than looking.** The CLI and the
  provisioner first waited with growing pauses, and overshot a claim that
  takes 0.2 s by as much again.
- **A pod's user needs a name.** Running the pod as user 1000 in an image
  that has no such user broke `coder ssh`. The pool's pods run as root in a
  user namespace, which the Restricted standard allows.
- **Agent Sandbox refuses labels it does not know on a claim.** Only its
  own label domain is allowed by default. Nothing here relies on labels.

## Limits

- **The workload can read its pod's token and its daemon's session.** They
  are the same principal. A copy of the session works from anywhere for as
  long as the workload lives, with the workload's scopes. In a daemon's
  sandbox there was nothing to copy.
- **A pod that dies without a word leaves a live record.** When a pod is
  deleted, its daemon reports that the workload stopped. When a node is
  lost, nobody does: the identity can no longer be used, because its
  credential was in the pod, but the workload stays listed until its
  workspace is removed. Not tried.
- **coderd must reach the cluster's API.** It is configured from the
  environment with one cluster and one namespace. Several clusters would
  need the review to go to the right one, or coderd to verify tokens with
  the clusters' public keys instead.
- **The cluster's answer about a token is reused for 30 seconds.**
- **Only coderd's built-in provisioners build sandbox templates.** External
  provisioner daemons do not.
- **A pool costs what its idle pods cost**, and a burst beyond it waits
  for pods to start. The pods fetch the daemon when they start, so a pool
  must be replaced when coderd is upgraded.
- **Every run creates and deletes a workspace.** That means two builds in
  the audit log, a notification to template administrators, and a name
  that must be free among the sponsor's workspaces. Quota is not counted.
- **No stronger runtime was tried.** The cluster has no gVisor or Kata.
  The pods are ordinary containers in a user namespace.
- **The disk is scratch.** It is gone with the pod.
- **The web UI was looked at, not tested.** Creating a workspace from a
  sandbox template in the browser was not tried.
- **The sandbox provisioner is a shortcut.** It stands in for Terraform.
  [DESIGN.md](DESIGN.md) has ways to define the pool in Terraform and to do
  without it, with what each would cost.
- **One cluster, one coderd replica, one day.**
