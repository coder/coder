# Where the pool should be defined

The spike in this directory defines the warm pool with `kubectl`, and builds
a workload's workspace with a provisioner that runs no Terraform. Both are
shortcuts. This note collects ways to define the pool in Terraform and to
leave the Terraform provisioner alone. Nothing in it is built unless it
says so.

## What the spike showed, and what is separable

Three things made a workload in its own pod fast and attested. None of
them depends on the sandbox provisioner:

- **Pod identity.** A daemon authenticates as the pod it runs in, with the
  kubelet's token, and coderd finds it by the pod's UID as its instance
  ID. An ordinary Terraform template can register that UID with the
  existing `coder_agent_instance` resource. The provider's list of
  authentication types needs one more entry.
- **The daemon fetches its workload.** The command starts before coderd has
  a connection to a daemon that has just been bound.
- **Something was running before it was asked for.** Here that was an Agent
  Sandbox warm pool. It could be anything that is provisioned ahead of
  time.

The sandbox provisioner did one job: bind a pod that already runs to a new
workspace without running Terraform. That is the part to replace.

## What a foundation is good for here

[Foundations](https://app.notion.com/p/406d579be592804a93ebeb621ae3f162)
are Terraform-managed shared infrastructure with a lifecycle of their own,
whose outputs templates read. Whatever binds workloads to pods, a
foundation is the right owner of what the pods share:

- **The namespace, the pods' service account and the network policy.**
  Changing what agents may reach, for every template at once and without
  rebuilding anything, is the PRD's own first goal.
- **The pool itself**, in the ideas below that keep Agent Sandbox: its pod
  template, size, limits and runtime class. Applying the foundation with a
  new image replaces the idle pods, so workloads get the image without a
  template rollout.
- **The trust anchor.** A foundation can output what coderd needs to
  believe a pod's token: the cluster's issuer and signing keys, the
  audience, the namespace and the service account. coderd then verifies
  tokens offline and needs no credential for it. Each foundation is one
  trust domain, which also answers "several clusters".
- **coderd's credential for the cluster**, where coderd makes claims. The
  foundation creates the narrow service account and exports its token as a
  sensitive output, instead of an administrator copying one into coderd's
  environment.

A foundation is not the place for the per-run step. It is applied by an
operator, rarely, and deliberately does not react to workspaces.

Two things the PRD does not have yet would be needed: coderd reading a
foundation's outputs for itself, where today only templates do, and a
template reading them at claim time without a Terraform run.

## Ideas

### 1. Terraform claims a pod for every run

- **The foundation** defines the Agent Sandbox pool.
- **The template** is ordinary Terraform: a `SandboxClaim` manifest, an
  agent with pod identity, and a `coder_agent_instance` with the claimed
  pod's UID.
- **A run** is a workspace build: Terraform plans and applies to claim,
  and again to give the pod up.

Nothing is subverted, and almost nothing is new: one authentication type
in the provider. This works today, with
[template-terraform/main.tf](template-terraform/main.tf) and the pool of
this spike: the workspace's daemon is vouched for by the cluster, and
`coder run` in the workspace takes 0.2 s to 0.3 s.

It is the slowest, because every run waits for a build. How slow depends
almost entirely on the resource that makes the claim, not on Terraform.
See [Measured](#measured). A resource made for claiming, in the Coder
provider or one of its own, would bring a run to a few seconds at most.

### 2. Prebuilt workspaces are the pool, and claiming one needs no build

Coder already has a pool that Terraform builds: prebuilt workspaces. A
template preset says how many to keep, coderd builds them ahead of time
with the ordinary provisioner, and a new workspace claims one.

- **The pool is defined in the template**: a preset with a number of
  prebuilt instances. Terraform builds every pod and destroys every pod.
- **The foundation** holds what the pods share.
- **Claiming** is the new part. Today a claim runs Terraform once more, to
  apply what depends on the new owner. The daemon keeps its token and is
  told to start over as the new owner's. A template that declares nothing
  owner-dependent could be claimed by changing the workspace's owner and
  name in the database, with no build.
- **A run** is a claim, then the workload, then an ordinary delete build
  in the background.

Agent Sandbox becomes optional. The template may still use its `Sandbox`
resource, for a runtime class or for volumes that survive a stop, but the
warm pool is Coder's.

What it costs: two Terraform runs per workload, off the path the user
waits on, so the provisioners must keep up with the rate of runs. A pool
refills at the speed of a build, not of a controller. Prebuilt workspaces
are a licensed feature.

What must be decided: how a template declares that a claim changes
nothing, and what the daemon learns when its workspace changes owner.

Tried, with today's claim: the template of idea 1 with a preset that keeps
two prebuilt workspaces. Pod identity needed no change. A prebuilt
workspace's daemon is vouched for by the cluster, and after the claim the
same pod answers as the new owner's workspace. The claim was no faster
than building, though. See [Measured](#measured).

### 3. Every developer's workspace carries its own small pool

The template of an ordinary workspace declares, next to the developer's
pod, a pool of one or two sandbox pods. `coder run my-workspace -- ...`
claims from that workspace's pool. This is the first design of this work
again, where the template declared the shape of workload pods, with a pool
in place of a pod that is started on demand.

- **The pool is defined in the template** and lives and dies with the
  workspace. No foundation is needed, though one may hold the policy.
- **Workloads belong to a workspace again**, as in the daemon's Docker
  sandboxes. The pods can mount the developer's volume, which a shared pool
  cannot offer.
- **coderd claims**, and needs the cluster credential for it.

What it costs: idle pods per running workspace instead of per deployment,
and the workload is not a workspace.

### 4. The pool is a thing Coder knows, and workloads are not workspaces

The foundation's Terraform declares the pool to Coder with a new provider
resource, next to the Kubernetes objects that make it. `coder run --pool`
claims from it. No workspace is created, so no provisioner is involved in
a run at all.

- **Fastest and cheapest per run**: no workspace, no builds, no
  notifications, nothing to delete but the claim.
- **Least reuse**: shell access, ports and the web UI hang off workspaces
  today. A workload's daemon would need them without one.

This fits one-shot commands at a high rate. It gives up "a workload is a
workspace".

## How they compare

|                             | 1. Terraform claims    | 2. Prebuilt and claimed             | 3. Pool per workspace    | 4. Pool without workspaces |
|-----------------------------|------------------------|-------------------------------------|--------------------------|----------------------------|
| Pool is defined in          | A foundation           | The template's preset               | The template             | A foundation               |
| The pod is started by       | The controller         | Terraform                           | The controller           | The controller             |
| Terraform runs per workload | Twice, one in the way  | Twice, out of the way               | None                     | None                       |
| A workload is a workspace   | Yes                    | Yes                                 | No                       | No                         |
| Needs Agent Sandbox         | Yes                    | No                                  | Yes                      | Yes                        |
| Works off Kubernetes        | No                     | Yes, with another proof of identity | No                       | No                         |
| The developer's files       | No                     | No                                  | Yes                      | No                         |
| New in Coder                | An authentication type | A claim without a build             | A claim by coderd        | A pool Coder knows         |
| Time for `-- true`          | 3 s to 14 s, measured  | Under a second, expected            | Under a second, expected | 0.8 s, as this spike       |

## Measured

What a run waits for, on the cluster and machine of
[RESULTS.md](RESULTS.md). Each line is `coder create` for a workspace
whose pod comes from the same warm pool, three runs.

| The workspace is built by                                                  | Create           | Delete         |
|----------------------------------------------------------------------------|------------------|----------------|
| The sandbox provisioner of this spike                                      | 0.53 s to 0.89 s | 0.1 s to 0.2 s |
| Terraform, with an agent and nothing else                                  | 0.84 s to 0.86 s | 0.6 s          |
| Terraform, claiming with shell commands that stand in for a claim resource | 3.2 s to 4.0 s   | 1.2 s          |
| Terraform, claiming with the Kubernetes provider's manifest resource       | 8.1 s to 13.8 s  | 5.8 s to 7.3 s |
| Terraform, claiming a prebuilt workspace of that template                  | 6.2 s to 7.1 s   | 6.3 s to 7.9 s |

- **Terraform itself is not slow.** A build that touches no cloud takes
  under a second.
- **The generic manifest resource is.** It asks the cluster for its
  schemas when it plans.
- **The shell stand-in** runs `kubectl` four times, which is most of its
  three seconds. A resource in a provider would not.

- **A prebuilt workspace's claim is a build that changes nothing here.**
  Terraform planned "0 to add, 0 to change, 0 to destroy" and still took
  six seconds, for the same reason as the line above it. With a warm pool
  behind the template, a prebuilt workspace saves nothing: the pod was
  running already.
- **After a claim the daemon was slow to follow.** The first `coder run`
  in a claimed workspace took 5.7 s to 8.4 s, because the daemon of this
  spike is not told that its workspace has a new owner. It finds out when
  its connection is dropped.

After the build, `coder run` in such a workspace took 0.2 s to 0.3 s.

## Recommendation

Idea 1 first, then idea 2, with a foundation for what the pods share.

- **Idea 1 is nearly free and already works.** It needs the pod identity
  from this spike and one entry in the provider. With a resource made for
  claiming it should cost a second or two more than the sandbox
  provisioner, and it removes that provisioner.
- **Idea 2 is how to get back under a second** without bringing the
  shortcut back. It is the same template: a prebuilt workspace of it has
  claimed its pod already, and claiming the workspace changes its owner.
  It also makes the result useful beyond agents, since any template whose
  resources do not depend on the owner could then be claimed that fast.
- **Ideas 3 and 4 give up "a workload is a workspace".** Idea 3 is worth
  keeping in mind for sharing a developer's files.

The next things to find out: how fast a claim resource in a provider is,
and whether a claim without a build holds up, which turns on what in a
template may depend on the owner.
