# Runtime workload daemons on Kubernetes

A proof of concept, behind the `runtime-workloads` experiment.

A workspace template declares two workspace daemons: one for the human, and a
**shape** that defines what a daemon for workloads looks like. `coder run`
provisions a new daemon from the shape at runtime, in its own pod, without a
workspace build. Each runtime daemon proves who it is with a token Kubernetes
issued to its pod. Coder never gives it a credential.

Workloads are usually AI agents, but nothing here is specific to them.

| File                                 | Purpose                                                 |
|--------------------------------------|---------------------------------------------------------|
| [template/main.tf](template/main.tf) | A workspace template with a human daemon and a shape    |
| [operator.yaml](operator.yaml)       | The workload operator and the service accounts it needs |
| [validate.py](validate.py)           | Checks a running setup, including masquerade attempts   |
| [RESULTS.md](RESULTS.md)             | What the checks showed, and the limits of the design    |

## How it works

```text
coder run dev -- <command>            (a logged-in human)
   |
   v
coderd  records a pending workload for the workspace's shape
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
        UID to the workload, and serves it as that identity only
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
- **The identity** is `<workspace UUID>/pod/<pod UID>`. It is associated with
  the workspace owner and records who requested it. coderd binds a workload
  to a pod only when the operator reports that it created that pod.
- **The command** given to `coder run` is delivered by Coder as the new
  daemon's startup script. It is not in the pod spec.
- **In Coder** a runtime daemon is a workspace agent whose parent is the
  shape's agent, so it belongs to the workspace build and disappears with it.
- **In the web UI** the shape is hidden, and the workspace page lists the
  running workloads with their command, identity and launch time.

Templates without a shape, and clusters without an operator, behave as they
do today.

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

```sh
coder whoami                                  # the human
coder run dev -- coder identity               # a new daemon prints its identity
coder run dev --name agent-1 -- coder whoami  # no user session, workload identity
coder run --list dev
coder exp rpty dev.agent-1                    # a terminal in the workload
coder run --remove agent-1 dev
kubectl -n coder-runtime-workloads get pods
```

A workload lives until it is removed or the workspace stops.

## Validate

```sh
CODER_URL=https://coder.example.com \
CODER_SESSION_TOKEN="$(coder tokens create --name validate --lifetime 1h)" \
python3 validate.py
```

The script provisions workloads, tries to make one pass for another, and
checks revocation and the operator's permissions. It takes a few minutes.
Add `--rotation` to also wait ten minutes for a pod token to expire and be
rotated. Its docstring lists the other settings. It scales the operator down
and up, creates and deletes pods, and stops and starts the workspace, so run
it against a throwaway setup.

## Clean up

```sh
coder delete dev --yes
kubectl delete namespace coder-runtime-workloads
```
