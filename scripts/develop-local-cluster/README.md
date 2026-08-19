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

## Bootstrap a Lima VM with mTLS

Use `setup-local-cluster-lima.sh` to prepare a fresh, apt-based Lima VM and deploy the complete local environment. The script expects rootful Docker, installs missing development tools, clones Coder into the guest's writable `~/src/coder` directory, creates the cluster, adds a license, and configures AI Gateway to connect to Coder with mTLS.

Create a Lima VM from macOS:

```console
limactl start \
  --name coder-dev \
  --cpus 6 \
  --memory 16 \
  --disk 100 \
  template:docker-rootful
```

Run the bootstrap script inside the VM. Supply the license using a file when possible:

```console
CODER_DEV_LICENSE_FILE="$HOME/coder-license.jwt" \
  ./scripts/develop-local-cluster/setup-local-cluster-lima.sh
```

You can instead set `CODER_DEV_LICENSE` or omit both variables to enter the license at a hidden interactive prompt. The script removes `CODER_DEV_LICENSE` from the subprocess environment before running installers and build commands.

The bootstrap script:

1. Installs missing base packages such as Git, Make, OpenSSL, jq, and vim.
2. Installs checksum-pinned mise, kubectl, and k9s.
3. Uses mise to install the repository-pinned Go, Node.js, pnpm, Helm, and kind versions.
4. Clones or updates Coder in `~/src/coder` and checks out `pawel/develop-local-cluster`.
5. Runs `develop-local-cluster.sh` to deploy PostgreSQL and Coder.
6. Adds the license before deploying the Premium provisioner and AI Gateway components.
7. Generates a development CA, Coder server certificate, and AI Gateway client certificate.
8. Configures AI Gateway to use Coder's HTTPS service with mTLS.
9. Verifies that Coder rejects a client without a certificate, accepts the generated client certificate, and reports AI Gateway ready.

Common bootstrap overrides:

| Environment variable             | Default                             |
|----------------------------------|-------------------------------------|
| `CODER_REPO_URL`                 | `https://github.com/coder/coder`    |
| `CODER_REPO_REF`                 | `pawel/develop-local-cluster`       |
| `CODER_REPO_DIR`                 | `~/src/coder`                       |
| `CODER_DEV_CLUSTER_NAME`         | `coder-local`                       |
| `CODER_DEV_CLUSTER_NAMESPACE`    | `coder`                             |
| `CODER_DEV_CLUSTER_GATEWAY_PORT` | `4001`                              |
| `CODER_DEV_CLUSTER_BUILD_JOBS`   | `2`                                 |
| `KUBECTL_VERSION`                | `v1.35.0`                           |
| `K9S_VERSION`                    | Latest release at installation time |
| `MTLS_CERT_DAYS`                 | `30`                                |
| `MTLS_VALIDATION_PORT`           | `30443`                             |

Set `CODER_DEV_CLUSTER_BUILD_JOBS=1` if the VM is still under memory pressure during the Vite production build.

The generated certificates and Helm values are stored under:

```text
.coderv2/clusters/coder-local/mtls
```

The mTLS validation uses a temporary loopback-only port-forward and removes it when validation completes. The normal development Coder URL remains `http://127.0.0.1:3000`; that HTTP endpoint does not require a client certificate. This setup validates and protects the AI Gateway-to-Coder HTTPS connection, but it does not disable Coder's HTTP service.

If a fresh bootstrap fails after creating the cluster, the script removes the new cluster and its local resources. It does not delete a cluster that existed before the script started.

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

Use flags or `CODER_DEV_CLUSTER_*` environment variables to override cluster name, namespaces, ports, password, starter template, build concurrency, and other settings. Flags take precedence over environment variables. Coder builds default to two parallel jobs to avoid exhausting memory in development VMs. Set `CODER_DEV_CLUSTER_BUILD_JOBS` or `--build-jobs` to tune this limit.

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
