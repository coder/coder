# Runtime workload daemons on Kubernetes

A proof of concept, behind the `runtime-workloads` experiment.

A workspace template declares two workspace daemons: one for the human, and a
**shape** that defines what a daemon for workloads looks like. `coder run`
provisions a new daemon from the shape at runtime, in its own pod, without a
workspace build. Each runtime daemon proves who it is with a token Kubernetes
issued to its pod. Coder never gives it a credential.

Every workload gets its own **workload identity** in Coder, sponsored by the
user who ran it. By default that identity can do nothing in Coder. `coder run
--scope` grants it a subset of its sponsor's permissions.

Workloads are usually AI agents, but nothing here is specific to them.

| File                                 | Purpose                                                 |
|--------------------------------------|---------------------------------------------------------|
| [template/main.tf](template/main.tf) | A workspace template with a human daemon and a shape    |
| [operator.yaml](operator.yaml)       | The workload operator and the service accounts it needs |
| [validate.py](validate.py)           | Checks a running setup, including masquerade attempts   |
| [RESULTS.md](RESULTS.md)             | What the checks showed, and the limits of the design    |

## How it works

```text
coder run dev --scope <scope> -- <command>      (the workspace owner)
   |
   v
coderd  creates a workload identity: sponsor, workspace, scopes
        records a pending workload for the workspace's shape
   ^
   |  sync every 2s (the operator dials out; coderd needs no cluster access)
   |
workload operator (in the workspace's namespace)
   |  clones the shape Deployment's pod template into one bare pod
   v
pod "workload-<name>"  runs the workspace daemon, with the unchanged init script
   |  presents its own pod token on every request
   v
coderd  verifies the token with the cluster's public keys, maps the pod
        UID to the workload identity, and serves it as that identity only:
        as its sponsor, within its scopes
```

- **The shape** is a `coder_agent` with `CODER_WORKLOAD_SHAPE=true` in its env
  and a `kubernetes_deployment_v1` with `replicas = 0` and the label
  `coder.com/workload-shape=true`. The Deployment only holds the pod template
  and owns the pods. No pod ever runs the shape itself.
- **The operator** is `coder exp workload-operator`, run from the same binary
  a workspace agent downloads. It uses a namespaced Role: read Deployments,
  manage pods.
- **A workload pod** is a clone of the shape with three changes: the shape's
  `CODER_AGENT_TOKEN` is removed, a projected service account token for
  coderd's audience is mounted, and the pod runs as a service account with no
  permissions and no Kubernetes API token.
- **The workload identity** is a record in Coder that permissions attach to.
  [Identity and permissions](#identity-and-permissions) describes it.
- **The command** given to `coder run` is delivered by Coder as the new
  daemon's startup script. It is not in the pod spec.
- **In Coder** a runtime daemon is a workspace agent whose parent is the
  shape's agent, so it belongs to the workspace build and disappears with it.
- **In the web UI** the shape is hidden, and the workspace page lists the
  running workloads with their command, identity, sponsor, scopes and launch
  time.

Templates without a shape, and clusters without an operator, behave as they
do today.

## Identity and permissions

Every workload has a **workload identity**: a record in Coder, created when
`coder run` is called and before anything runs. It holds three things.

- **A sponsor.** The user account whose permissions bound what the workload
  may do. The sponsor is the workspace owner: nobody else can run a workload
  in a workspace, not even an administrator. Any account can be a sponsor,
  including a service account, so a bot can run workloads in its own
  workspaces.
- **Scopes.** API token scope names, such as `workspace:read`, given with
  `coder run --scope`. They are fixed when the workload is requested.
- **A binding.** coderd binds the identity to a pod only when the operator
  reports that it created that pod. The pod then proves it holds the identity
  with its pod token. The identity is displayed as
  `<workspace UUID>/pod/<pod UID>`.

A workload presents its pod token to Coder's API, the same one its daemon
connects with. It acts as its sponsor, narrowed to its scopes. No API token
is created and nothing is stored in the pod. Inside a workload the `coder`
CLI needs no login.

| Granted with `coder run`                       | What the workload can do in Coder                               |
|------------------------------------------------|-----------------------------------------------------------------|
| nothing                                        | Nothing. Lists are empty, and its own workspace is not found.   |
| `--scope template:read`                        | List and read the templates its sponsor can read.               |
| `--scope workspace:read --scope template:read` | Also list and read the workspaces its sponsor can read.         |
| any scope                                      | Never run or remove workloads, and never more than its sponsor. |

- **Sponsor-relative.** The same scopes on a member's workload list only that
  member's workspaces. On an administrator's workload they list every
  workspace.
- **No sponsor credentials.** A workload never receives its sponsor's Git SSH
  key or external auth tokens, whatever its scopes.
- **Listing workspaces needs both read scopes**, as it does for a scoped API
  token: Coder leaves out a workspace whose template the caller cannot read.
- **Revocation.** Removing the workload, deleting its pod or stopping the
  workspace revokes the identity. The next request with its token is refused.

`coder identity` inside a workload shows the record:

```text
Workload identity: 229fc06b-3998-4b40-b1e5-983fbe102d0b/pod/2990fe99-0875-46f4-b932-819cfc0d3b0a
  Kind:          runtime daemon, attested by its Kubernetes pod
  Daemon:        none2 (2ea97d76-6a27-48c3-b179-247bb578f7fb)
  Workspace:     alice/alice-dev (229fc06b-3998-4b40-b1e5-983fbe102d0b)
  Identity ID:   c5bced56-0c75-4976-87ba-90dd49a176de
  Sponsor:       alice (8658bc21-eeae-4b87-b6de-45aaa630c1f5)
  Scopes:        none
  Shape:         workload
  Pod:           coder-runtime-workloads/workload-none2 (2990fe99-0875-46f4-b932-819cfc0d3b0a)
```

## Set up

You need a Kubernetes cluster, and `coder` built from this branch for both
the server and the CLI.

1. Start coderd with the experiment. Pods must be able to reach its access
   URL. Run a single replica: operator state is held in memory.

   ```sh
   CODER_EXPERIMENTS=runtime-workloads coder server
   ```

2. Create the namespace and the operator's credentials. The token must belong
   to an organization administrator, and the URL must be coderd's access URL.

   ```sh
   kubectl create namespace coder-runtime-workloads
   kubectl -n coder-runtime-workloads create secret generic coder-operator \
     --from-literal=url=https://coder.example.com \
     --from-literal=token="$(coder tokens create --name workload-operator --lifetime 168h)"
   ```

3. Install the operator.

   ```sh
   kubectl -n coder-runtime-workloads apply -f operator.yaml
   ```

4. Push the template and create a workspace. The provisioner needs a
   kubeconfig that can create Deployments and ConfigMaps in the namespace.

   ```sh
   coder templates push runtime-workloads -d template
   coder create dev --template runtime-workloads
   ```

   The template's variables set the kubeconfig path and context, the
   namespace and the image.

## Try it

Run these as the owner of the workspace.

```sh
coder whoami                                  # the human
coder run dev -- coder identity               # a new daemon prints its identity
coder run dev --name agent-1 -- coder whoami  # no user session, workload identity
coder exp rpty dev.agent-1                    # a terminal in the workload
coder run --remove agent-1 dev
kubectl -n coder-runtime-workloads get pods
```

Permissions:

```sh
coder run dev -- coder list                   # no scopes: no workspaces found
coder run dev -- coder templates list         # no scopes: no templates found
coder run dev --scope template:read -- coder templates list
coder run dev --scope workspace:read --scope template:read -- coder list
coder run --list dev                          # each workload's sponsor and scopes
```

A workload lives until it is removed or the workspace stops.

## Validate

```sh
CODER_URL=https://coder.example.com \
CODER_SESSION_TOKEN="$(coder tokens create --name validate --lifetime 1h)" \
python3 validate.py
```

The script provisions workloads, tries to make one pass for another, and
checks revocation, the operator's permissions, and what workloads with and
without scopes can do. It takes a few minutes.
Add `--rotation` to also wait ten minutes for a pod token to expire and be
rotated. Its docstring lists the other settings, including two optional
workspaces: an ordinary single-agent one (`PLAIN_WORKSPACE`), and one that
another user owns (`OTHER_WORKSPACE`), to check that only the owner can run
workloads in it. It scales the operator down
and up, creates and deletes pods, and stops and starts the workspace, so run
it against a throwaway setup.

## Clean up

```sh
coder delete dev --yes
kubectl delete namespace coder-runtime-workloads
```
