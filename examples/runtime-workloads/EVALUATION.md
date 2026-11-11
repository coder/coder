# Evaluation

A review of this proof of concept from the seat of a CISO and a CTO at the
kind of organization that runs Coder for regulated work: a bank, a
government department, an intelligence agency. It covers the branch as built
and tested on 2026-11-11. Check IDs such as `C7` refer to
[validate.py](validate.py) and [RESULTS.md](RESULTS.md).

## Verdict

The architecture is the right one. A sandbox that holds no credential, a
single mediated channel to the control plane, permissions that start at
nothing and never exceed the sponsor's, policy that the template's author
sets, and a durable record of every run: these are the properties a security
team asks for, and they are cheap here. A workload starts in about 1.5
seconds and needs nothing installed in the cluster.

It is not finished. The answer depends on who asks:

| Reviewer                       | Answer today                                                                                        |
|--------------------------------|-----------------------------------------------------------------------------------------------------|
| CTO                            | Yes to the direction. Fast, no new infrastructure, existing templates untouched.                    |
| CISO of a bank                 | A pilot, with engineers' own agents, no network and read scopes. Not production with write scopes.  |
| CISO of an intelligence agency | No. Shared-kernel isolation, an owner who can act as their own agents, and unmediated model access. |

The four things that stand between the pilot and production are, in order:
model access without keys, mediated egress, central policy, and a complete
activity record. They are described under
[What would make it undeniable](#what-would-make-it-undeniable).

| Concern                                  | State                                                                |
|------------------------------------------|----------------------------------------------------------------------|
| A workload passing for another principal | Holds, tested from inside and outside                                |
| Credentials to steal from a sandbox      | None exist                                                           |
| Least privilege in Coder                 | Good default; scopes are coarse; no central ceiling                  |
| Attribution of changes                   | Good: audited, and marked as the workload's                          |
| Attribution of reads                     | Weak: request log at debug level only                                |
| Record of what ran                       | Present; the outcome is the daemon's word                            |
| Isolation of the sandbox                 | A container on a shared kernel; no stronger tier built               |
| Egress control                           | All or nothing                                                       |
| Model access for agents                  | Not mediated; designed, not built                                    |
| Resilience of long-running agents        | Weak: they die with the daemon and with autostop                     |
| Operability at scale                     | Thin: no metrics, quotas or retention                                |
| Adoption by template authors             | Hard: a hand-written pod manifest and a cluster with user namespaces |

## What holds up

These were attacked and held.

- **No masquerade.** A workload that sends another workload's identity in
  headers, cookies, bearer tokens and query parameters is still itself
  (`C1`). Going around the daemon gets nothing (`C7`). From outside, only
  the one daemon that runs a workload can vouch for it: not the human's
  daemon in the same pod (`F4`), not another workspace's daemon, not a user
  token (`C9`, `C10`).
- **Nothing to steal.** A sandbox holds the address of its channel and
  nothing else (`C2`). A prompt-injected agent cannot leak a Coder token,
  because it never had one.
- **Default deny, three times.** No permissions in Coder (`D1`), no network
  (`E1`), no files of the workspace (`E5`). Each is granted per run, and
  each grant is on the record (`E2`).
- **Bounded by the sponsor.** A workload acts within its sponsor's
  permissions and its own scopes (`D3` to `D5`), cannot be given all of its
  sponsor's permissions (`E4`), cannot start or remove workloads (`D5`), and
  never receives the sponsor's Git SSH key or external auth tokens.
  Suspending the sponsor stops their workloads at once (Go test).
- **The template's author is in control of the sandbox.** Image, network and
  shared directories are decided by the daemon's settings, which only the
  template sets (`E3`, `E6`). A user cannot widen them from the CLI.
- **Blast radius of an escape is limited by placement.** The pod is not
  privileged and its root is an unprivileged user on the node (`F1`). The
  daemon that runs workloads is not the human's: an escaped workload does
  not land next to the human's home directory or the human daemon's token
  (`F2`), and the token it does find cannot fetch the owner's Git SSH key
  (`F3`).
- **An identity cannot outlive its workload.** Removal, exit, a killed
  sandbox and a stopped workspace all end it at once (`G1`, `G2`, `G4`,
  `G5`), and the record remains (`G6`).
- **Accountability for changes.** Starts, refusals, ends and terminal
  sessions are audit entries. A change made through the API by a workload
  is audited under the sponsor and marked with the workload's identity (Go
  test).
- **Small footprint.** No operator, service account, role or secret (`H1`,
  `H2`). There is no new component to patch or to trust.

## What is missing

- **Model access.** An agent needs a model. Today the only ways to give it
  one put a provider key or a Coder token inside the sandbox, which is what
  the design exists to avoid. AI Gateway does not yet accept workload
  identities. This is the largest gap for the main use case. See
  [DESIGN.md](DESIGN.md#agents).
- **Mediated egress.** The network is `none` or `outbound`. A real agent
  needs a package mirror, Git and a model, and nothing else. With
  `outbound`, a sandbox reaches whatever the pod reaches, including ports
  the human listens on in the same pod, and none of it is logged per
  workload.
- **Git.** A workload has no way to authenticate to a Git host. It can
  change a shared checkout, and the human pushes.
- **Secrets.** There is no way to hand a workload a secret. A secret typed
  into the command line is stored in the record, the audit log and the web
  UI.
- **Central policy.** A sponsor may grant any scope they hold. An
  organization cannot cap the scopes of workloads, restrict who may run
  them, require approval for write scopes, or limit their number or
  lifetime. The limit of 16 workloads per workspace is a constant.
- **Object-level scopes.** Scopes limit the kind of action, not the object.
  `workspace:update` covers every workspace of the sponsor.
- **A record of reads.** The audit log records changes. Reads appear only
  in coderd's request log, at debug level.
- **A record of what happened inside.** Coder keeps the command, the last
  16 KB of output and the exit code. It does not record processes, file
  changes, network connections, or what was typed into a terminal.
- **A stronger isolation tier.** Containers share the kernel of the
  workspace pod. The runtime is an interface, but only Docker is behind it.
  An agency running hostile or unreviewed code wants a microVM. This
  cluster has no hardware virtualization, so none could be tried.
- **A kill switch.** One workload can be removed, and one sponsor
  suspended. There is no way to stop every workload of an organization.
- **Retention.** Records are kept forever. There is no purge and no export.

## What is unreliable

- **Workloads die with their daemon.** A daemon that restarts, for an
  upgrade or a crash, removes its sandboxes. An agent three hours into a
  task is gone, and the record says `stopped` or `lost`.
- **Autostop does not know about workloads.** A workspace whose human went
  home stops on schedule and takes its agents with it.
- **The outcome is the daemon's word.** Start, sponsor, scopes and API
  activity are recorded by coderd itself. Exit code and output are reported
  by the daemon, and root in its container could report anything.
- **No resource limits per workload.** Docker runs without cgroups in a
  user-namespaced pod, so one workload can starve the others in the same
  workspace. The human's container has its own limits and is not affected.
- **Docker's storage is ephemeral.** Images are pulled again after every
  workspace start. An image that is not the default is pulled on first use,
  and the registry is then in the critical path of `coder run`.
- **End-of-life reporting can lag.** The daemon tries three times to report
  an ending. After that, coderd learns of it the next time someone lists
  the workspace's workloads. The identity is useless in between, because
  its sandbox is gone, but the record is stale.
- **Tested narrowly.** One cluster, one coderd replica, one container
  runtime, one day. User namespaces for pods are recent, and managed
  Kubernetes offerings differ in whether they allow them.

## What is undercooked

- **The owner can act as their own workloads** (`C11`). The owner can open
  a shell in the daemon's container and use its token. They gain nothing,
  since a workload never exceeds its sponsor, but "the agent did it" and
  "I did it" are the sponsor's word. For most organizations the sponsor is
  accountable either way. An agency will want the vouching daemon out of
  the owner's reach.
- **An escape moves sideways.** A workload that escapes its container can
  use the daemon's token to act as its sibling workloads, including ones
  with wider scopes. It stays within the same sponsor and the same
  workspace.
- **The sandbox host is not a hardened container.** It has `CAP_SYS_ADMIN`
  inside its user namespace and runs without seccomp or AppArmor profiles,
  because Docker needs that there. Most clusters will need a policy
  exemption for it. A kernel bug reachable from a user namespace is the
  remaining way to the node.
- **Image policy is by name.** Patterns match image names, not digests, and
  signatures are not verified.
- **Writable shares are a way back to the human.** A workload that can
  write to a shared checkout can plant a hook or a build script that the
  human later runs. Shares are read-only by default for that reason.
- **The audit log was not seen running.** It needs a license. The entries
  are asserted in a Go test against the audit interface. How the audit page
  displays them was not checked.
- **Output that was kept may hold secrets.** The 16 KB tail is stored in
  the database and shown to anyone who may read the workspace.
- **Multiple coderd replicas** should work, since all state is in the
  database and daemons are reached over the tailnet, but this was not run.

## What is not ergonomic

- **For users.** Flags must come before the workspace and `--`. Listing,
  logs and removal are flags of `coder run` and not commands of their own.
  A workload without the right scopes sees empty lists and "not found", with
  no hint that a scope is missing. Scope names must be known by heart. There
  is no `--env`, `--workdir` or resource flag.
- **For files.** Only what lies under a directory the template names can be
  shared, not the home directory. The human is root in their container and
  workloads are user 1000, so a writable share needs its permissions opened
  by hand.
- **In the web UI.** Workloads can be seen, not started, stopped, followed
  or attached to.
- **For template authors.** The example is a raw Deployment manifest with
  two agents, eighteen capabilities and four security settings. The second
  agent shows up in the workspace page and in SSH configuration. This needs
  to be a registry module or a provider resource before anyone adopts it.
- **For operators.** There are no metrics, no usage numbers, and no page
  that lists workloads across workspaces. The audit log is the only
  deployment-wide view.

## What would make it undeniable

In the order that buys the most trust.

1. **Model access through the channel.** AI Gateway accepts a request that
   a daemon vouches for, authorizes it as the sponsor, and attributes every
   prompt, tool call and token to the workload identity. A harness is
   pointed at `$CODER_URL` and holds no key. This turns "an agent with no
   credentials" from a slogan into a fact, and gives the security team
   per-agent model policy and spend. Design in
   [DESIGN.md](DESIGN.md#agents).
2. **Egress through the channel.** The daemon offers an HTTP proxy on the
   channel with an allow-list from the template, and logs each connection
   under the workload's identity. Git goes the same way, with the sponsor's
   external auth applied by coderd behind a `git` scope, so that the token
   never enters the sandbox. `none` then becomes the only network anyone
   needs, and every byte that leaves is attributed.
3. **Central policy.** Workloads become a permission of their own: who may
   run them, in which templates, with which scopes at most, how many, for
   how long. Write scopes can require a second person. One switch stops
   every workload of an organization.
4. **A complete record.** Every request a workload makes, reads included,
   goes to a workload activity log that can be exported. Full output goes
   to object storage. Terminal sessions can be recorded. Records expire by
   policy.
5. **Assurance tiers under one identity model.** A microVM runtime for
   untrusted code. A workload host the owner cannot enter, so that vouching
   is the platform's word and not the owner's. For the highest tier, the
   pod-per-workload design from the first rounds of this work, where the
   cluster attests each workload, behind the same `coder run` and the same
   identities.
6. **Durable agents.** Sandboxes are re-adopted after a daemon restart, a
   running workload holds off autostop, and a workload's disk can be kept
   and attached to a later one. Design in
   [DESIGN.md](DESIGN.md#disk-snapshots).
7. **A product surface.** A provider resource or registry module for the
   workload host, a `coder workloads` command group, controls and a
   terminal in the web UI, metrics, and Coder Tasks running on workloads.

Items 1 to 4 are what a bank's CISO would ask for before production. Item 5
is what an agency would ask for before a pilot.

## What changed in this round

The previous state of this branch failed this review on more points. Since
then:

| Before                                                     | Now                                                                       |
|------------------------------------------------------------|---------------------------------------------------------------------------|
| The workspace pod was privileged                           | No privileged container; the pod's root is unprivileged on the node       |
| Workloads ran next to the human, under one daemon          | A daemon and container of their own, without the owner's Git key          |
| Every sandbox had outbound network                         | No network by default; outbound on request if the template allows         |
| Any image could be named                                   | The template's image, plus an allow-list                                  |
| A sandbox could not see any file of the workspace          | Directories the template allows can be shared, read-only by default       |
| A workload vanished when it ended                          | A record with outcome and output; a filter in the UI and CLI              |
| Nothing was audited                                        | Start, refusal, end and terminal attach; changes marked as the workload's |
| A workload could be given all of its sponsor's permissions | Refused                                                                   |
| Output only                                                | `coder run -it` gives a terminal in the sandbox                           |
| Only the `coder` CLI could use the channel                 | Any HTTP client, through `$CODER_URL`                                     |
