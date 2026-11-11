# Workloads inside a workspace daemon

A proof of concept, behind the `runtime-workloads` experiment.

`coder run` runs a command as a **workload**: in a sandbox that the
workspace's own daemon creates, in about a second, without a workspace build
and without a new pod. Every workload gets a **workload identity** of its
own in Coder, sponsored by the user who ran it. By default that identity can
do nothing in Coder. `coder run --scope` grants it a subset of its sponsor's
permissions.

A workload holds no credential. Its sandbox has one way to Coder, through the
daemon, and the daemon vouches for whichever workload a request came from.
One workload cannot pass for another, for the daemon, or for the human.

Workloads are usually AI agents, but nothing here is specific to them.

| File                                 | Purpose                                              |
|--------------------------------------|------------------------------------------------------|
| [template/main.tf](template/main.tf) | A workspace whose daemon can run workloads in Docker |
| [validate.py](validate.py)           | Checks a running setup, from inside and outside      |
| [RESULTS.md](RESULTS.md)             | What the checks showed, and the limits of the design |

## How it works

```text
coder run dev --scope <scope> -- <command>      (the workspace owner)
   |
   v
coderd  creates a workload identity: sponsor, scopes, and the one daemon
        that may vouch for it
   |    asks that daemon to run the command (over the tailnet, like the
   v    rest of the daemon's API)
workspace daemon
   |    its runtime starts a sandbox and gives it a private channel
   v
sandbox  runs the command; holds no credential
   |    every request to Coder goes through the channel
   v
workspace daemon  strips whatever credentials the request carries, adds its
   |              own agent token and the identity the channel belongs to
   v
coderd  checks that the token is the daemon's, that the identity is
        assigned to that daemon and not revoked, and serves the request as
        the sponsor, within the identity's scopes
```

- **The runtime** is what isolates workloads, and it is pluggable. A runtime
  starts a sandbox and serves it a channel that nothing else can reach. The
  one built here is Docker: a container per workload, with its own
  filesystem, processes and network, no capabilities and an unprivileged
  user. The channel is a unix socket in a directory mounted read-only into
  that container alone. A microVM runtime would use a virtual socket or
  network device, and a namespace-only runtime a private network interface.
  See [`agent/agentworkload/runtime.go`](../../agent/agentworkload/runtime.go).
- **The daemon** is the ordinary workspace agent, started with
  `CODER_AGENT_WORKLOAD_RUNTIME=docker`. It needs a Docker daemon that shares
  its filesystem.
- **The identity** is a record in Coder: a sponsor, scopes, and the daemon
  that runs the workload. It is displayed as
  `<workspace UUID>/workload/<identity UUID>`. It lasts as long as the
  command: when the command exits or the workload is removed, it is revoked.
- **The sponsor** is the workspace owner. Nobody else can run a workload in
  a workspace, not even an administrator. Any user account can be a sponsor,
  including a service account.
- **Scopes** are API token scope names, such as `workspace:read`. Without
  scopes a workload can see nothing. Whatever its scopes, it cannot run or
  remove workloads, cannot use the daemon's own API, and never receives its
  sponsor's Git SSH key or external auth tokens.
- **Inside a sandbox** the `coder` CLI needs no login. The daemon mounts the
  CLI and sets `CODER_WORKLOAD_SOCKET`; the CLI then talks to the channel.
- **In the web UI** the workspace page lists the running workloads with
  their command, identity, sponsor, scopes and launch time.

Workspaces whose daemon has no runtime behave as they do today, and
`coder run` on them is refused with a clear message.

## What a workload can and cannot do

| Granted with `coder run`                       | What the workload can do in Coder                             |
|------------------------------------------------|---------------------------------------------------------------|
| nothing                                        | Nothing. Lists are empty, and its own workspace is not found. |
| `--scope template:read`                        | List and read the templates its sponsor can read.             |
| `--scope workspace:read --scope template:read` | Also list and read the workspaces its sponsor can read.       |
| any scope                                      | Never run or remove workloads, never more than its sponsor.   |

- **Sponsor-relative.** The same scopes on a member's workload list only
  that member's workspaces. On an administrator's workload they list every
  workspace.
- **Not inherited.** A workload does not get its daemon's permissions, its
  sponsor's, or another workload's.
- **Listing workspaces needs both read scopes**, as it does for a scoped API
  token: Coder leaves out a workspace whose template the caller cannot read.

## Who is trusted

- **The workspace daemon vouches for its workloads.** Root in the workspace,
  and so its owner, can act as any workload in it. The owner is the sponsor,
  so this gains no permissions; it only blurs which of them did something.
- **A workload is not trusted at all.** But it is only as contained as its
  runtime contains it. A workload that escapes a Docker sandbox becomes root
  in the workspace.
- **A daemon can vouch only for its own workloads.** Another workspace's
  daemon, a user's API token, or a request with no credential cannot.

## Set up

You need a Kubernetes cluster that allows privileged pods, and `coder` built
from this branch for both the server and the CLI.

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
   namespace and the images. Nothing else is installed in the cluster.

## Try it

Run these as the owner of the workspace.

```sh
coder run dev -- coder identity               # who the workload is
coder run dev -- coder whoami                 # no user session
coder run dev -- sh -c 'echo hi; exit 3'      # output and exit code come back
coder run dev -d --name agent-1 -- sleep 600  # detached
coder run --list dev
coder run --remove agent-1 dev
```

Permissions:

```sh
coder run dev -- coder list                   # no scopes: no workspaces found
coder run dev --scope template:read -- coder templates list
coder run dev --scope workspace:read --scope template:read -- coder list
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
it. It takes about a minute. Its docstring lists the settings, including an
optional second workspace that another user owns. It stops and starts the
workspace, so run it against a throwaway setup.

## Clean up

```sh
coder delete dev --yes
kubectl delete namespace coder-hosted-workloads
```
