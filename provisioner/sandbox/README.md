# Native sandbox provisioner prototype

This opt-in provisioner uses Coder workspaces, jobs, quotas, and agent connections with disposable containerd/gVisor compute.
The readiness target is p95 below five seconds from a workspace-create request to a successful command through Coder.
That target requires measurement on the configured Linux host; unit and simulated runtime tests are not latency evidence.

## One-host setup

Use a dedicated Linux/amd64 host with at least four available vCPUs and 8 GiB of memory for four concurrent 1 vCPU/2 GiB sandboxes, plus capacity for the host and Coder services.
Run the provisioner directly on the host with permission to use containerd, create network namespaces, and execute CNI plugins.
All four workers must share the same persistent state directory.
Do not attach another host with the `sandbox_host=local` tag to this deployment.

Install containerd 2.x with the overlayfs snapshotter, the CNI `bridge`, `host-local`, and `loopback` plugins, and a pinned gVisor release containing `runsc` and `containerd-shim-runsc-v1`.
Follow the [gVisor installation guide](https://gvisor.dev/docs/user_guide/install/) and [containerd integration guide](https://gvisor.dev/docs/user_guide/containerd/quick_start/).
Keep any `gvisor-bin` sidecar directory beside `runsc` as required by that release.
Both binaries must be visible in the containerd service's `PATH`.
The provisioner also resolves `runsc` from its administrator-controlled `PATH` for bounded verification and cleanup of runtime allocations that outlive containerd metadata.
The fixed namespace uses the shim's `/run/containerd/runsc/coder-sandbox` runtime root; reserve that root for this prototype.
The runtime uses `io.containerd.runsc.v1` directly, so CRI runtime configuration alone does not configure this client.
It writes a private `runsc.toml` in the state directory and passes its path to the shim to select `systrap` explicitly.
An existing file with different contents or public permissions is rejected.
No KVM device is required by this platform.

Copy `testhost/10-coder-sandbox.conflist` into a dedicated `/etc/coder-sandbox/cni` directory.
Choose a subnet that does not overlap your host network, and replace the sample DNS server with a resolver reachable from that subnet.
Loopback DNS addresses do not work inside the sandbox.
Enable IPv4 forwarding and allow the bridge's forwarding and masqueraded traffic through the host firewall.
The Coder access URL must be reachable from this bridge; a loopback-only access URL does not work.

The runtime socket is never mounted into a sandbox.
The prototype uses bridge networking and does not add a tenant egress policy; configure host firewall rules for the test deployment's network policy.

## Curated image and fixed template

Build the Coder agent from this checkout, then build and publish the curated image to an administrator-controlled registry.
Set `BASE_IMAGE` to a verified digest of the Go base image if you need reproducible image construction.
The final image reference in every template must use a digest.

```shell
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=slim -o build/coder-sandbox-agent ./cmd/coder
docker build --platform linux/amd64 -f provisioner/sandbox/testhost/Dockerfile -t REGISTRY/coder-sandbox:experiment .
docker push REGISTRY/coder-sandbox:experiment
```

Record the resulting `REGISTRY/coder-sandbox@sha256:...` reference as `SANDBOX_IMAGE` on the host.
Load and unpack it in the dedicated namespace before creating workspaces:

```shell
sudo ctr --namespace coder-sandbox images pull --platform linux/amd64 "$SANDBOX_IMAGE"
sudo ctr --namespace coder-sandbox images check
```

Start does not pull images or unpack missing image layers.
It fails before allocating compute if the exact image reference is absent or not unpacked with overlayfs.
Measure pull and unpack time separately when evaluating cache misses.
Every successful start still creates a new container, writable snapshot, and network namespace.

Create a template directory containing this `sandbox.yaml`, replacing the image placeholder with the recorded digest:

```yaml
version: 1
image: REGISTRY/coder-sandbox@sha256:REPLACE_WITH_64_HEX_DIGEST
cpu: 1
memory_mib: 2048
workdir: /workspace
daily_cost: 1
```

The schema rejects unknown fields, extra YAML documents, mutable image references, invalid limits, and non-absolute working directories.
The image must contain the working directory and allow its configured user to write there.
CPU, memory, working directory, and cost default to the values above when omitted.
Only administrators author these fixed templates; user variables, rich parameters, dynamic expressions, and placement overrides are rejected.

## Enable and use

For a local development test deployment on the sandbox host, use the repository development launcher:

```shell
export CODER_PROVISIONER_TYPES=sandbox
export CODER_PROVISIONER_DAEMONS=4
export CODER_API_RATE_LIMIT=20000
export CODER_SANDBOX_STATE_DIRECTORY=/var/lib/coder-sandbox
export CODER_SANDBOX_CNI_CONFIG_DIRECTORY=/etc/coder-sandbox/cni
export CODER_SANDBOX_CNI_BIN_DIRECTORY=/opt/cni/bin
./scripts/develop.sh --port 3020 --web-port 6020 --starter-template= --access-url http://HOST_IP:3020
```

Run the provisioner with the host privileges described above and set the deployment's access URL to an address reachable from sandboxes.
The increased API limit is for the benchmark deployment's frequent status polling; keep the normal policy on other deployments.
The default containerd address is `/run/containerd/containerd.sock`; override it with `CODER_SANDBOX_CONTAINERD_ADDRESS`.
The default state directory is `/var/lib/coder-sandbox` and must be private to the provisioner account.
The containerd namespace is fixed at `coder-sandbox`.

For a deployment with Terraform workers, run four dedicated external workers on the same sandbox host using the enterprise Coder binary from this checkout:

```shell
coder provisioner start --provisioner sandbox --name sandbox-1
```

Repeat with distinct names `sandbox-2` through `sandbox-4` and the same state directory.
Use the existing supported provisioner authentication mechanism.
A provisioner key must already carry `sandbox_host=local`; user or pre-shared-key authentication adds that tag automatically.
A sandbox worker cannot advertise other backends.
Terraform workers retain their existing defaults.

Import with the modified CLI, then use the existing workspace creation, terminal, and SSH interfaces:

```shell
coder templates push native-sandbox --provisioner sandbox --directory /path/to/template
coder create agent-sandbox --template native-sandbox
coder ssh agent-sandbox -- true
coder ssh agent-sandbox -- sandbox-smoke
```

The corresponding template-version API accepts `provisioner: "sandbox"` with the uploaded tar or zip source.
Import and dry-run validate the manifest and calculate quota without opening containerd or creating compute.
No Terraform executable is needed on either the API or worker host for these templates.

## Lifecycle and recovery

Apply persists intent and a fresh agent credential before runtime changes.
Retries of the same build reuse that credential and allocation identity.
Apply returns as soon as the runtime task starts; job completion registers the agent, after which normal Coder connectivity can become ready.
Waiting for agent connectivity inside Apply would prevent registration.

Stop and delete remove the task, container, writable snapshot, CNI allocation, and network namespace.
Restart creates a clean environment with a new credential.
Workspace scheduling supplies automatic stopping.
Orphan deletion is rejected before a job is queued.

The host stores versioned operation records under `operations/<workspace UUID>/<build UUID>.json` and network ownership under `network/<allocation ID>` in the state directory.
Treat the operation records as credentials and retain the directory across provisioner restarts.
The workspace lock and monotonic build number fence both replayed requests and previously unseen delayed requests.
Allocations carry workspace, build, build-number, and image labels.
Inventory recovery occurs under that lock on the next authorized lifecycle operation, including when Coder's opaque state is empty.

Cancellation attempts cleanup with an independent ten-second deadline.
Cleanup verifies both containerd and the independent runsc inventory before removing writable data or network ownership.
Matching OCI identity annotations authorize deletion of a surviving runtime after containerd loses its task record.
An unresolved task-creation intent retains its owner and network when absence alone cannot prove that a late create is impossible.
The intent records the host boot ID; after a host reboot, a normal authorized lifecycle operation can resolve an absent allocation because no creator from the earlier boot can still run.
Failed cleanup retains its record for a retry or a subsequent authorized lifecycle operation.
Worker shutdown does not remove running or uncertain allocations.
If the local operation journal is lost while compute remains, replay of the same start is rejected because the original credential cannot be recovered from runtime labels.
A new authorized build can replace or delete that allocation.
Do not prune operation records during the experiment, because retired records retain the generation fence after compute is removed.
There is no autonomous host-failure recovery or background garbage collector in this prototype.

## Validation and latency

Run the benchmark from a client near the test deployment after the image cache is prepared:

```shell
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=YOUR_TEST_ACCOUNT_TOKEN
go run ./scripts/sandbox-benchmark -template TEMPLATE_UUID -output sandbox-results.json
```

The default run performs 100 sequential creations and 25 bursts of four concurrent creations.
If the deployment enforces workspace quotas, give the test account budget for at least four concurrent sandboxes.
Each sample includes the create request, queueing, provisioning, job completion, agent connection, and an authenticated SSH command.
The JSON report contains individual samples, p50/p95/p99, failures, server provisioner timings, and separate serial and burst outcomes.
Deletion and optional repository preparation occur outside the readiness timer.
Use `-prepare-command sandbox-smoke` to exercise shell, local Git clone/commit, Go builds/tests, C compilation, Node, and Python after readiness.
Use a representative repository clone/build command in another run to measure repository preparation separately.

Before accepting the milestone, also verify browser terminal input/output, remote Git access under the deployment's credentials, cancellation during creation, worker termination around container/task creation, repeated deletion, and cleanup with missing Coder state.
Run an existing Terraform template on a Terraform worker as a regression check.
Record host capacity, image digest, kernel, containerd/runsc versions, network topology, percentile results, and every failed request.
Do not treat a passing simulated-runtime test, Docker emulation, or a cache-miss run as proof of the five-second target.

Persistent storage, snapshots, Docker-in-Docker, arbitrary end-user images, multi-host placement, and a separate public Sandbox API are outside this experiment.
