# Design notes

What this proof of concept is meant to grow into. Nothing in this file is
built unless it says so. [EVALUATION.md](EVALUATION.md) explains why these
are the next steps.

## One primitive for commands and agents

A workload is a command in a sandbox with an identity. An agent is a
workload whose command is an agent harness. There is no second mechanism.

| What runs                    | How                                                                           | State                                   |
|------------------------------|-------------------------------------------------------------------------------|-----------------------------------------|
| A one-off command            | `coder run dev -- make test`                                                  | Built                                   |
| A long-running process       | `coder run -d --name indexer dev -- ./index`                                  | Built                                   |
| A shell                      | `coder run -it dev -- sh`                                                     | Built                                   |
| An agent with a terminal UI  | `coder run -it --image <agent image> --mount /projects/repo:rw dev -- claude` | Mechanics built, not run with a harness |
| A headless agent             | `coder run -d --image <agent image> dev -- claude -p "..."`                   | Mechanics built, not run with a harness |
| An agent driven from the web | A native session whose harness runs in a sandbox                              | Designed below                          |

What the mechanics give an agent today: a terminal, a working directory
shared with the human, an image the template allows, an identity, scoped use
of Coder's API from any HTTP client, and a record of the run. What they do
not give it is a model. That is the first thing to build.

## Agents

### Model access through the channel

A harness needs a model endpoint and insists on some credential for it. The
aim is that the endpoint is Coder's AI Gateway and the credential is the
workload identity, so that no key of any kind is in the sandbox.

```text
harness in the sandbox
   |  ANTHROPIC_BASE_URL=$CODER_URL/api/v2/ai-gateway/anthropic
   |  OPENAI_BASE_URL=$CODER_URL/api/v2/ai-gateway/openai/v1
   |  a placeholder where the client demands a key
   v
loopback address -> channel -> workspace daemon
   |  removes every credential header, adds its agent token and the
   |  workload identity (as it does for every request today)
   v
coderd, in front of the AI Gateway
   |  verifies the vouch exactly as the API does; requires a scope for
   |  model use; marks the request as delegated by that workload identity
   v
AI Gateway
      authorizes as the sponsor; records the interception, its prompts,
      tool calls and tokens under the workload identity; applies the
      sponsor's budget and the provider keys that only the gateway holds
```

The pieces, and what exists already:

- **In the sandbox.** The program that starts the command already offers
  the channel on a loopback address and sets `CODER_URL` (built). It would
  also set the two base URLs and placeholders when the workload has the
  scope.
- **In the daemon.** The channel already replaces credentials on every
  request (built). It must also remove `X-Api-Key` and
  `X-Coder-AI-Governance-Token`, which the gateway reads and the API does
  not.
- **In coderd.** The gateway has a precedent for a caller that is vouched
  for and not authenticated by a key: Coder's own chat service reaches the
  gateway in-process with a delegated API key ID, and the gateway then only
  checks that the key is alive
  (`coderd/aibridge/factory.go`, `coderd/aibridgedserver`). A workload is
  the same case with a different voucher. A middleware in front of the
  gateway's data plane resolves the vouch with the function the API uses
  (`vouchedWorkloadIdentity` in `coderd/workloads.go`) and puts the
  workload identity on the request. The gateway's authorization step
  accepts it, checks that the workload is not over and its sponsor is
  active, and returns the sponsor as the owner.
- **In the record.** Interceptions get a nullable `workload_identity_id`
  next to the initiator. The gateway's session views, usage and budgets
  then answer "which agent" as well as "which user".
- **A scope.** Model use needs a scope of its own, so that a workload with
  no scopes still cannot spend its sponsor's budget.

Why it is not built here: the gateway's HTTP routes are registered by the
enterprise package and need a license, so they do not exist on this
deployment and the change could not be run.

### Native sessions

The branch `feat/native-workspace-agents` drives Claude Code, Codex and
OpenCode from the web UI. A session there is a harness process that the
workspace agent starts and talks to, shown in the browser over the Agent
Client Protocol through a relay in coderd. Read against this design:

| On that branch                                                                    | With workloads                                                 |
|-----------------------------------------------------------------------------------|----------------------------------------------------------------|
| Harnesses are child processes of the agent, as the same user                      | Each session's harness runs in its own sandbox                 |
| Sessions share the home directory and every provider credential                   | A sandbox holds no credential and sees only shared directories |
| A session has no owner and no record in Coder                                     | A session is a workload: sponsor, scopes, record, audit        |
| Model access is a provider key or gateway token copied into the workspace by hand | Model access through the channel, as above                     |
| Any user who may connect to the workspace drives every session                    | Only the sponsor attaches to a workload                        |

The fusion keeps that branch's session manager and harness adapters and
changes where they run:

- **The manager runs inside the sandbox**, next to the harness, as the
  workload's command. The adapters prove that a harness process is theirs
  by user ID, process namespace and files, so they must share a sandbox
  with it. Running only the harness in a sandbox would mean rewriting those
  checks.
- **One sandbox per session.** Sessions then stop sharing a harness server,
  a home directory and a connection pool.
- **The protocol crosses the channel.** A sandbox can only dial out, so the
  manager holds a connection open to its daemon on a path the daemon keeps
  to itself, and the daemon pairs it with the browser's connection that
  coderd relays. `coder run -it` already carries a terminal along the same
  three hops (browser or CLI, coderd, daemon, sandbox), so the transport is
  proven.
- **Opening the native terminal UI** is attaching to the workload.
- **A session that ends keeps its disk**, so that it can be resumed. That
  is the next section.

What is left of the trust question from that branch is the same as for any
workload: the daemon vouches, and the template decides what a sandbox may
be.

## Agent Sandbox

[Agent Sandbox](https://agent-sandbox.sigs.k8s.io) is a Kubernetes SIG Apps
project. Its `Sandbox` resource is one pod with a stable name, optional
persistent volumes, suspend and resume, and a scheduled end. Templates
describe sandboxes, and a warm pool keeps ready ones that a claim adopts.
Isolation is the pod's runtime class, such as gVisor or Kata Containers. The
API has been `v1beta1` since version 1.0 in August 2026.

It was not considered when this proof of concept moved workloads into the
workspace daemon. It should have been: the reason for that move was the cost
of starting a pod, and a warm pool is an answer to that cost.

It does not replace what is built here. Agent Sandbox manages sandboxes and
has no notion of whom a sandbox acts for. Its documentation calls the claim
labels it puts on a pod "informational, not proof of identity", and lists
identity among the things it is exploring. The identity, the sponsor, the
scopes, the channel, the record and the audit trail are Coder's, and stay
as they are. Agent Sandbox would be a second runtime under them.

|                       | Docker inside the workspace daemon (built)      | Agent Sandbox (not built)                                           |
|-----------------------|-------------------------------------------------|---------------------------------------------------------------------|
| Start                 | About 1.5 s, nothing scheduled                  | Fast from a warm pool; a pod start, or a node, when it is empty     |
| Isolation             | A container on the workspace pod's kernel       | A pod, and a sandbox runtime such as gVisor or Kata where installed |
| Resource limits       | None per workload                               | The pod's own                                                       |
| In the cluster        | Nothing                                         | A controller, four resource types, and idle pods in the pools       |
| Outside Kubernetes    | Works wherever the workspace has Docker         | Does not apply                                                      |
| The workspace's files | A bind mount, decided per run                   | A volume fixed when the pod is created, so not for a warm pod       |
| Who vouches           | A daemon the workspace owner can enter          | The cluster can, through a token bound to the pod                   |
| Keeping a disk        | Designed below                                  | Built in: volume templates, suspend and resume                      |
| Pod security          | Needs added capabilities and no seccomp profile | An ordinary pod                                                     |
| Maintained by         | Coder                                           | The Kubernetes community                                            |

How it would fit:

- **Creating the sandbox.** `coder run` ends in a claim against a pool that
  the platform team defines. Whatever creates the claim needs a Kubernetes
  permission to do so. That should not be a container the workspace owner
  can enter.
- **The channel.** A small Coder container in the sandbox pod serves the
  same unix socket to the workload's container through a shared directory,
  so the workload still holds no credential. It authenticates to coderd
  with the pod's own service account token, and coderd asks the cluster
  whether the token belongs to the pod that the claim adopted. The cluster
  then vouches, not a daemon in the owner's reach. A stolen token is worth
  one workload, not all the workloads of a workspace. This is the
  attestation the first rounds of this work used, without the operator
  written for them.
- **Policy.** Image, resources, runtime class and network policy move from
  daemon settings to sandbox templates. The Coder template names the
  templates its workspaces may claim from.
- **Files.** A warm pod cannot be given the workspace's checkout after the
  fact. Workloads that need it either start cold with the volume, or get
  the code another way, such as Git through the channel.

What it would give the review in [EVALUATION.md](EVALUATION.md): a stronger
isolation tier, per-workload limits, and a voucher out of the owner's reach,
which are three of the things an agency would ask for. What it would cost:
the property that nothing is installed in the cluster, and with an empty
pool the latency this design was built to avoid.

The two belong together as tiers behind one `coder run` and one identity
model: Docker in the daemon as the default that works everywhere, Agent
Sandbox where the cluster has it and the code must not share the
workspace's kernel.

Not tried here. The cluster has an Agent Sandbox controller that belongs to
another experiment and serves only its own namespace, and a second
controller would compete with it. It has no sandbox runtime class either.
That experiment's objects show a cold sandbox pod ready about 4 s after it
was created, and a claim bound to a warm one within the same second.

## Disk snapshots

A workload's disk should be able to outlive it, so that a later workload
can pick up where it stopped: resume an agent's session, look at what a
failed run left behind, or try two continuations of one state.

### Model

- **A snapshot is a record in Coder**, in the organization of the
  workspace, with the workload it was taken from, its sponsor, the runtime's
  reference to the data, a digest, a size and an expiry.
- **Attaching makes a new workload with a new identity.** An identity is
  the record of one run and is never reused. The new workload's record
  names the snapshot it started from, so the chain of runs can be followed.
- **Nothing is inherited but the disk.** Scopes, network and shared
  directories are granted anew by `coder run`, within today's policy. A
  snapshot taken by a workload with no network cannot become a way to
  carry data out later without someone granting a network.
- **Only the sponsor attaches**, and only in the workspace the snapshot was
  taken in, until there is a reason to share.

```sh
coder run --keep dev --name fix-1 -- agent ...       # keep the disk when it ends
coder run --from fix-1 dev -- agent --resume         # a new workload on that disk
coder run --from fix-1 -it dev -- sh                 # or look around in it
coder run --all dev                                  # shows which runs have a disk
coder run --forget fix-1 dev                         # delete the disk
```

### In the runtime

The runtime interface gets two additions: a sandbox can be snapshotted, and
a workload's spec can name a snapshot to start from.

- **Docker.** A snapshot is `docker commit` of the exited container into an
  image that only the daemon may name, and attaching is `docker run` from
  that image. Layers are copy-on-write, so a snapshot costs what the run
  changed, and several workloads can start from one snapshot. This was
  tried by hand in the pod of this proof of concept: a container that wrote
  20 MB was committed and a new sandbox, with the same restrictions,
  started from it and saw the files.
- **MicroVMs.** A snapshot is a snapshot of the VM's block device. A runtime
  that can also save memory could resume a workload mid-thought instead of
  restarting its command.

What a snapshot holds is the sandbox's own filesystem. It does not hold
shared directories, which belong to the workspace, or running processes.

### Where the data lives

| Place                                 | Survives a workspace stop  | Cost                                                                                                                    |
|---------------------------------------|----------------------------|-------------------------------------------------------------------------------------------------------------------------|
| Docker's storage in the pod, as today | No                         | Nothing to build. Good for retries within a session.                                                                    |
| The same on a persistent volume       | Yes                        | A volume per workspace. Images stay cached too, which makes `coder run` faster after a start.                           |
| An image registry or object store     | Yes, and across workspaces | The daemon must not hold the credential. coderd would take the upload over the daemon's connection and own the storage. |

The persistent volume is the first step. The third option is what "attach
from another workspace" would need.

### Risks to design for

- **A snapshot is data at rest** that an agent wrote. It needs an expiry, a
  quota and the same access rule as the workload's output.
- **A snapshot is a way to persist.** A workload that was subverted can
  leave something behind for the next run, which may have wider scopes. The
  chain of runs is on record, and policy can refuse to start from a
  snapshot with more scopes than the run that made it.
- **Root in the daemon's container can alter stored data.** coderd records
  the digest when the snapshot is taken and the daemon checks it when
  attaching, so that a change is at least noticed.

### With a checkout

For an agent that works on source code, the disk is the lesser half. The
checkout lives in a shared directory. The pattern that fits is a Git
worktree per workload in the shared directory, mounted writable into that
workload alone, with the main checkout mounted read-only or not at all.
The worktree is then the durable result that the human reviews, and the
snapshot holds the agent's own state. `coder run` could create and remove
the worktree itself.
