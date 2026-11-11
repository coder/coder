# Workloads inside a workspace daemon

A proof of concept, behind the `runtime-workloads` experiment.

`coder run` runs a command as a **workload**: in a sandbox that a daemon of
the workspace creates, in about a second, without a workspace build and
without a new pod. Every workload gets a **workload identity** of its own in
Coder, sponsored by the user who ran it. By default that identity can do
nothing in Coder, and its sandbox has no network and sees no files of the
workspace. `coder run` grants more, within what the sponsor may do and what
the workspace's template allows.

A workload holds no credential. Its sandbox has one way to Coder, through the
daemon, and the daemon vouches for whichever workload a request came from.
One workload cannot pass for another, for the daemon, or for the human.

What ran, for whom, with which permissions and how it ended stays on record
after the workload is gone, and is written to the audit log.

Workloads are usually AI agents, but nothing here is specific to them.

| File                                 | Purpose                                                    |
|--------------------------------------|------------------------------------------------------------|
| [template/main.tf](template/main.tf) | A workspace with a daemon that runs workloads in Docker    |
| [validate.py](validate.py)           | Checks a running setup, from inside and outside            |
| [RESULTS.md](RESULTS.md)             | What the checks showed                                     |
| [EVALUATION.md](EVALUATION.md)       | A security and operations review, with what is missing     |
| [DESIGN.md](DESIGN.md)               | Where this goes next: agents, model access, disk snapshots |

## How it works

```text
coder run dev --scope <scope> -- <command>      (the workspace owner)
   |
   v
coderd  creates a workload identity: sponsor, scopes, and the one daemon
        that may vouch for it; writes the start to the audit log
   |    asks that daemon to run the command (over the tailnet, like the
   v    rest of the daemon's API)
workspace daemon
   |    applies the template's policy: image, network, shared directories
   |    its runtime starts a sandbox and gives it a private channel
   v
sandbox  runs the command; holds no credential
   |    every request to Coder goes through the channel
   v
workspace daemon  strips whatever credentials the request carries, adds its
   |              own agent token and the identity the channel belongs to
   v
coderd  checks that the token is the daemon's, that the identity is
        assigned to that daemon and its workload is not over, and serves
        the request as the sponsor, within the identity's scopes
```

- **The runtime** is what isolates workloads, and it is pluggable. A runtime
  starts a sandbox and serves it a channel that nothing else can reach. The
  one built here is Docker: a container per workload, with its own
  filesystem, processes and network, no capabilities and an unprivileged
  user. The channel is a unix socket in a directory mounted read-only into
  that container alone. A microVM runtime would use a virtual socket or
  network device, and a namespace-only runtime a private network interface.
  See [`agent/agentworkload/runtime.go`](../../agent/agentworkload/runtime.go).
- **The daemon** is an ordinary workspace agent, started with
  `CODER_AGENT_WORKLOAD_RUNTIME=docker`. It needs a Docker daemon that shares
  its filesystem. In the example template it is a second agent in a
  container of its own, next to the human's.
- **The identity** is a record in Coder: a sponsor, scopes, and the daemon
  that runs the workload. It is displayed as
  `<workspace UUID>/workload/<identity UUID>`. It authorizes requests for as
  long as the command runs. After that it is the record of what ran.
- **The sponsor** is the workspace owner. Nobody else can run a workload in
  a workspace, not even an administrator. Any user account can be a sponsor,
  including a service account. Suspending the sponsor stops every one of
  their workloads from acting.
- **Scopes** are API token scope names, such as `workspace:read`. Without
  scopes a workload can see nothing. Whatever its scopes, it cannot have all
  of its sponsor's permissions, cannot run or remove workloads, cannot use
  the daemon's own API, and never receives its sponsor's Git SSH key or
  external auth tokens.
- **Inside a sandbox** nothing needs a login. The `coder` CLI talks to the
  channel. Other programs use `CODER_URL`, a loopback address that leads to
  the same channel, with any HTTP client and no token.
- **In the web UI** the workspace page always has a Workloads section, with
  each workload's command, identity, sponsor, scopes and times. A switch
  adds the workloads that are over, with how they ended.

Workspaces whose daemons have no runtime behave as they do today, and
`coder run` on them is refused with a clear message.

## What the template decides

A sponsor chooses within what the template's author allows. The daemon
enforces it, and what was granted is on the workload's record.

| Template variable           | Daemon setting                          | Default     | Meaning                                                    |
|-----------------------------|-----------------------------------------|-------------|------------------------------------------------------------|
| `workload_image`            | `CODER_AGENT_WORKLOAD_IMAGE`            | `alpine`    | The image of a sandbox that names none. Always allowed.    |
| `workload_allowed_images`   | `CODER_AGENT_WORKLOAD_ALLOWED_IMAGES`   | none        | Patterns of other images `coder run --image` may name.     |
| `workload_network`          | `CODER_AGENT_WORKLOAD_NETWORK`          | `none`      | `none`: no network at all. `outbound`: ordinary egress.    |
| `workload_allowed_networks` | `CODER_AGENT_WORKLOAD_ALLOWED_NETWORKS` | `outbound`  | Other networks `coder run --network` may ask for.          |
| `workload_allowed_mounts`   | `CODER_AGENT_WORKLOAD_ALLOWED_MOUNTS`   | `/projects` | Directories `coder run --mount` may share with a workload. |

- **Network.** A sandbox without a network still reaches Coder, because its
  channel is a file. Asking for less than the default is always allowed.
  Sandboxes can never reach each other.
- **Files.** A sandbox sees nothing of the workspace unless a directory is
  shared with it, and then read-only unless `:rw` is asked for. Symbolic
  links are resolved before the check. In the example template the human
  and the workloads share `/projects` and nothing else: not the human's home
  directory. The example keeps `/projects` on scratch storage that is
  emptied when the workspace stops.

## What a workload can and cannot do in Coder

| Granted with `coder run`                       | What the workload can do in Coder                             |
|------------------------------------------------|---------------------------------------------------------------|
| nothing                                        | Nothing. Lists are empty, and its own workspace is not found. |
| `--scope template:read`                        | List and read the templates its sponsor can read.             |
| `--scope workspace:read --scope template:read` | Also list and read the workspaces its sponsor can read.       |
| `--scope coder:all` or `--scope all`           | Refused. A workload exists to have less than its sponsor.     |
| any scope                                      | Never run or remove workloads, never more than its sponsor.   |

- **Sponsor-relative.** The same scopes on a member's workload list only
  that member's workspaces. On an administrator's workload they list every
  workspace.
- **Not inherited.** A workload does not get its daemon's permissions, its
  sponsor's, or another workload's.
- **Listing workspaces needs both read scopes**, as it does for a scoped API
  token: Coder leaves out a workspace whose template the caller cannot read.

## The record

Every workload leaves a record in Coder: name, command, sponsor, scopes,
image, network, shared directories, sandbox, start, end, why it ended, exit
code, and the last 16 KB of its output. `coder run --all` and the web UI
show the 50 most recent per workspace, and `coder run --logs` prints what
was kept of the output.

A workload ends in one of five ways:

| Status    | Meaning                                                            |
|-----------|--------------------------------------------------------------------|
| `exited`  | The command ran to its end. The exit code is recorded.             |
| `removed` | A user removed it.                                                 |
| `stopped` | Its daemon shut down, as it does when the workspace stops.         |
| `lost`    | Its sandbox disappeared, or its daemon went away without a word.   |
| `failed`  | The sandbox never started, for example because of a refused image. |

The daemon reports the end to coderd by itself. If it cannot, coderd notices
the next time anyone lists the workspace's workloads.

The [audit log](../../docs/admin/security/audit-logs.md) gets an entry when
a workload starts (also when it is refused), when it ends, and when a person
attaches a terminal to it. Each entry names the workspace, the daemon, the
sponsor, the command, the scopes and what the sandbox was given. Anything a
workload changes through Coder's API is audited as its sponsor's action
with the field `workload_identity_id`, so that an auditor can tell the
sponsor's workload from the sponsor in person. coderd's request log, which is written at debug level,
carries the same field on every request a workload makes, reads included.

## Who is trusted

- **The daemon that runs workloads vouches for them.** Root in its
  container, and so the workspace's owner, can act as any workload in it.
  The owner is the sponsor, so this gains no permissions; it only blurs
  which of the two did something.
- **A workload is not trusted at all.** But it is only as contained as its
  runtime contains it. A workload that escapes a Docker sandbox becomes root
  in the container that runs workloads. In the example template that is not
  the human's container: it does not hold the human's files or the human
  daemon's credential, and its own daemon cannot fetch the owner's Git SSH
  key or external auth tokens.
- **Nothing in the pod is root on the node.** The example template runs the
  pod in a user namespace and without `privileged`.
- **A daemon can vouch only for its own workloads.** The human's daemon,
  another workspace's daemon, a user's API token, or a request with no
  credential cannot.

[EVALUATION.md](EVALUATION.md) goes through what this does and does not
give a security team.

## Set up

You need a Kubernetes cluster with user namespaces for pods
(`hostUsers: false`), and `coder` built from this branch for both the server
and the CLI. On a cluster without user namespaces, the daemon can run in a
privileged pod next to `dind dockerd` instead, as an earlier version of this
template did, at the price of a privileged pod.

1. Start coderd with the experiment. Pods must be able to reach its access
   URL.

   ```sh
   CODER_EXPERIMENTS=runtime-workloads coder server
   ```

2. Create a namespace, push the template and create a workspace. The
   provisioner needs a kubeconfig that can create Deployments in the
   namespace.

   ```sh
   kubectl create namespace coder-hosted-workloads
   coder templates push hosted-workloads -d template
   coder create dev --template hosted-workloads
   ```

   The template's variables set the kubeconfig path and context, the
   namespace, the images and the policy above. Nothing else is installed in
   the cluster.

## Try it

Run these as the owner of the workspace.

```sh
coder run dev -- coder identity               # who the workload is
coder run dev -- coder whoami                 # no user session
coder run dev -- sh -c 'echo hi; exit 3'      # output and exit code come back
coder run -it dev -- sh                       # a terminal inside a sandbox
coder run dev -d --name agent-1 -- sleep 600  # detached
coder run --list dev                          # what runs
coder run --logs agent-1 dev                  # its output
coder run --remove agent-1 dev
coder run --all dev                           # what ran, and how it ended
```

Permissions in Coder:

```sh
coder run dev -- coder list                   # no scopes: no workspaces found
coder run dev --scope template:read -- coder templates list
coder run dev --scope workspace:read --scope template:read -- coder list
coder run dev --scope template:read -- sh -c 'wget -qO- $CODER_URL/api/v2/templates'
```

Network and files:

```sh
coder run dev -- wget -T 3 -qO- http://example.com                      # no network
coder run dev --network outbound -- wget -T 3 -qO- http://example.com
coder ssh dev.main -- sh -c 'mkdir -p /projects/repo && echo hello > /projects/repo/README'
coder run dev -- ls /projects                                           # not there
coder run dev --mount /projects/repo -- cat /projects/repo/README       # read-only
coder run dev --mount /projects/repo:rw -- touch /projects/repo/changed # needs a directory uid 1000 can write
```

A workload lives as long as its command.

## Validate

```sh
CODER_URL=https://coder.example.com \
CODER_SESSION_TOKEN="$(coder tokens create --name validate --lifetime 1h)" \
python3 validate.py
```

The script runs shell scripts as workloads to probe a sandbox from the
inside, and sends requests straight to coderd to play an attacker outside
it. It takes about 90 seconds. Its docstring lists the settings, including
an optional second workspace that another user owns. It stops and starts the
workspace, so run it against a throwaway setup.

The audit log entries are checked by the Go test `TestWorkloads` in
`coderd`, because the audit log needs a license to run.

## Clean up

```sh
coder delete dev --yes
kubectl delete namespace coder-hosted-workloads
```
