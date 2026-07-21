# develop-local-cluster

`develop-local-cluster` creates an isolated [kind](https://kind.sigs.k8s.io/) cluster for local Kubernetes development. It creates a local PostgreSQL StatefulSet from Coder's PostgreSQL image and deploys a locally built full Coder image, the external provisioner, and standalone AI Gateway from the source Helm charts in this repository.

The command is for local development only. It does not run in CI.

## Prerequisites

- Docker must be running.
- `kubectl`, Helm, Go, Make, and Git must be available.
- Install the repository's pinned tools, including kind:

  ```console
  mise install
  ```

The command supports Docker servers running on `amd64` or `arm64`.

## Quick start

Create the cluster:

```console
./scripts/develop-local-cluster.sh up
```

The first run deploys PostgreSQL and Coder, then creates an isolated Coder CLI session in `.coderv2/clusters/<cluster-name>`.

If Coder does not yet have the entitlements required by standalone AI Gateway and external provisioners, `up` offers to add a license interactively. Paste the license when prompted. When the license enables both features, the same command creates the component keys and deploys the provisioner and AI Gateway.

To add a license from a file instead, skip the prompt and then rerun `up`:

```console
./scripts/develop-local-cluster.sh up --no-license-prompt
./scripts/develop-local-cluster.sh coder licenses add -f ~/coder-license.jwt
./scripts/develop-local-cluster.sh up
```

Use the cluster-scoped `coder` command for all Coder CLI administration. It never uses or overwrites the worktree `.coderv2` session used by `scripts/develop.sh`.

```console
./scripts/develop-local-cluster.sh coder users list
```

## Inspect the cluster

`info` prints the exact kind context for the current worktree:

```console
./scripts/develop-local-cluster.sh info
```

Use that context with kubectl or k9s. For a predictable context name, set the cluster name when creating the cluster:

```console
./scripts/develop-local-cluster.sh --cluster-name coder-local up

kubectl --context kind-coder-local get pods --all-namespaces
k9s --context kind-coder-local
```

The command never changes the current kubectl context. Always pass `--context` when working from another terminal or worktree.

## Daily workflow

```console
# Build a fresh local Coder image and upgrade all installed Coder workloads.
./scripts/develop-local-cluster.sh reload

# Apply source Helm chart changes without building an image.
./scripts/develop-local-cluster.sh reload --charts-only

# Print the cluster name, context, URLs, current image, and bootstrap stage.
./scripts/develop-local-cluster.sh info

# Remove unused local Coder images for this cluster.
./scripts/develop-local-cluster.sh clean-images

# Delete the cluster, database, workspace resources, cluster CLI state, and
# unused local images for this cluster.
./scripts/develop-local-cluster.sh down

# Run down followed by a fresh up. The new database needs a license again.
./scripts/develop-local-cluster.sh reset
```

There is no restart command. Use k9s or kubectl to delete a pod, scale a Deployment, or start a Kubernetes rollout restart when needed.

## Configuration

Defaults are suitable for one worktree:

| Setting                 | Default                     |
|-------------------------|-----------------------------|
| kind cluster            | `coder-dev-<worktree-hash>` |
| control-plane namespace | `coder`                     |
| workspace namespace     | `coder-workspaces`          |
| Coder URL               | `http://127.0.0.1:3000`     |
| AI Gateway URL          | `http://127.0.0.1:4001`     |

Use flags or `CODER_DEV_CLUSTER_*` environment variables to override cluster name, namespaces, ports, password, starter template, and other settings. Flags take precedence over environment variables.

Pass additional Helm values after generated development values with repeatable flags:

```console
./scripts/develop-local-cluster.sh up \
  --coder-values ./local-coder-values.yaml \
  --provisioner-values ./local-provisioner-values.yaml \
  --gateway-values ./local-gateway-values.yaml
```

Changing host ports for an existing cluster requires `down` first because kind port mappings are configured when the cluster is created.

## Isolation and cleanup

Every Kubernetes and Helm command targets the exact `kind-<cluster-name>` context. The command does not use the current kubectl context, create port-forward processes, expose PostgreSQL on the host, or modify the normal worktree Coder CLI session.

`down` is idempotent. It deletes the named kind cluster, which removes Kubernetes resources, PVC data, workspace pods, secrets, and images loaded into the kind node. It then removes the matching cluster-specific CLI state and unused host-side local Coder images.

Use `--keep-on-failure` if you want to inspect a new cluster in k9s after a bootstrap failure. Without it, failures before Coder becomes healthy delete a newly created cluster automatically.
