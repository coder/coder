# Workspace-hosted workloads POC

Run a command in a prepared project environment without creating a workspace or running Terraform for each command:

```sh
coder run --template workload-host --cpu 2 --ram 4GiB --cmd 'go test -count=1 ./coderd/util/ptr ./coderd/util/slice'
```

This is an opt-in prototype for trusted colleagues, not a production serverless service. The first backend is Linux Docker with OverlayFS and cgroup v2, **not a microVM**. There is no UI.

## What runs where

```text
coder run
  -> coderd: organization/template authorization and host selection
  -> existing agent tailnet HTTP transport, dedicated supervisor credential
  -> workspace agent: local admission, durable dispatch guard, Docker supervisor
  -> private container: prepared image, repository, dependencies, command
```

Terraform still provisions the outer service workspace. An administrator explicitly registers its agent as a workload host. Ordinary template users can submit workloads without receiving host SSH access. Keep the service workspace administrator-owned and do not share its interactive access with workload users.

Every workload gets a private Docker writable layer, including its worktree, home, dependency state, and temporary files.
The image is pinned by content ID before admission.
The command is passed as an argument to the configured shell inside the container.
Caller environment isn't inherited automatically, but you can supply explicit values with `--env KEY=value`.
Host directories, host agent credentials, and the Docker socket are not passed to it.

CPU means a whole-number CFS quota, not exclusive cores. RAM accepts integer `MiB` or `GiB`, at least `16MiB`. Fractions, implicit units, millicores, signs, overflow, and silent rounding are rejected. All four flags are required. Admission includes a per-run memory overhead allowance; unused reservations are not offered to another workload.

Workloads use Docker's default bridge network with internet access enabled.
Operators manage firewall policy outside this POC, including restrictions on private networks, metadata endpoints, the host, and other containers.

## Use the sample template

As an administrator, you can use [`main.tf`](main.tf) to provision a dedicated Docker-in-Docker workspace and prepare its project image automatically.
The outer workspace has eight CPUs and 16 GiB of RAM, with a workload budget of four CPUs and 8 GiB.
You can adjust these values in `main.tf` before creating the template.

> [!WARNING]
> The host workspace is privileged and gives its owner administrative access to the machine running Docker.
> Keep this template private and restrict it to trusted operators.
> The POC doesn't separate permission to submit workloads from permission to create a workspace using the same template.

Your provisioner needs access to a Linux Docker daemon that supports privileged containers, `overlay2`, and cgroup v2.
The host workspace needs network access to your Coder server, Docker Hub, Alpine and Ubuntu package repositories, GitHub, Go module proxies, npm registries, and the tool and browser download hosts used by Coder during preparation.
The template never mounts the outer Docker socket into the workspace.

Use a server and CLI built from this branch with the `workloads` experiment enabled.
The agent downloads its binary from that server.
For a local development deployment, use `./scripts/coder-dev.sh` in place of `coder` in the following commands.

1. Initialize the template providers from the repository root:

   ```sh
   terraform -chdir=examples/workloads init
   ```

1. Create a private template in your organization:

   ```sh
   coder templates create workload-host --org "$ORG_ID" \
     --directory examples/workloads --private --agents-allowed=false --default-ttl 0
   ```

   This uses `templates create`, although deprecated, because it supports private access at creation time.
   `templates push` creates new templates with access for everyone by default.
   If your provisioner uses a different Docker socket, add `--variable docker_socket=unix:///path/to/docker.sock`.
   If Coder runs on the Docker host, add `--variable coder_url=http://127.0.0.1:3000` to avoid downloading the agent through a public tunnel.
   The template maps loopback addresses to `host.docker.internal` inside the container for both agent downloads and connections.

1. Create an administrator-owned host workspace:

   ```sh
   coder create workload-host --org "$ORG_ID" --template workload-host \
     --stop-after 0 --automatic-updates never
   ```

   Initial preparation can take tens of minutes and uses substantial disk space for development tools and dependency caches.
   [`host/Dockerfile`](host/Dockerfile) supplies Docker and the Bash bootstrap scripts.
   Terraform uploads the project [`Dockerfile`](Dockerfile) into the workspace.
   The bootstrap clones the pinned public `coder/coder` revision, installs its locked mise toolchain, Go modules, frontend dependencies, PostgreSQL, native build tools, and Chromium, then runs representative Go and frontend unit tests before starting the agent.
   A failed build prevents agent startup.
   Once connected, the agent has a prepared image and a private supervisor configuration.

1. Find the connected agent in the current workspace build, with `ORG_ID` set to your organization's UUID:

   ```sh
   WORKSPACE=workload-host
   set -o pipefail
   AGENT_ID=$(coder list --output json | jq -er --arg workspace "$WORKSPACE" --arg org "$ORG_ID" '
     [.[] | select(.name == $workspace and .organization_id == $org and .latest_build.status == "running")
       | .latest_build.resources[].agents[]? | select(.name == "host" and .status == "connected") | .id]
     | if length == 1 then .[0] else error("Expected one connected host agent") end
   ')
   ```

   Coder assigns an agent ID to each workspace build.
   Don't use `coder_agent.host.id` from Terraform as the registration ID.

1. Register the host from your local shell, with `CODER_URL` and an administrator's `CODER_SESSION_TOKEN` set for this deployment:

   ```sh
   set -o pipefail
   coder ssh "$WORKSPACE" -- cat /var/lib/coder-workload-host/registration.json |
     jq --arg agent "$AGENT_ID" '. + {agent_id: $agent}' |
     curl --fail-with-body --silent --show-error \
       -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
       -H 'Content-Type: application/json' \
       --data-binary @- \
       "$CODER_URL/api/v2/organizations/$ORG_ID/workload-hosts"
   ```

   The response identifies the registered agent, template, and prepared generation.
   The administrator's session token stays in your local shell, not the workspace or Terraform state.
   The supervisor token is generated inside the host and retained in its private state volume.

1. Run the prepared project's tests:

   ```sh
   coder run --org "$ORG_ID" --template workload-host \
     --cpu 2 --ram 4GiB --cmd 'go test -count=1 ./coderd/util/ptr ./coderd/util/slice'
   ```

   Both Go packages should pass with exit status zero. To exercise the prepared frontend dependencies:

   ```sh
   coder run --org "$ORG_ID" --template workload-host \
     --cpu 2 --ram 4GiB --cmd 'pnpm --dir site test src/utils/fileSize.test.ts --maxWorkers=2'
   ```

The prepared checkout is `/work/coder`, at the revision pinned in `Dockerfile`.
Toolchains, modules, `node_modules`, and browser downloads live in the image; there is no clone or dependency installation per workload.
The sample tests don't cover every Coder integration test: tests needing Docker services, external APIs, or credentials still require you to provide them.

The template persists Docker images and private supervisor state in separate named volumes across workspace stop/start.
It reuses the pinned image and supervisor token on a clean restart, without rebuilding or fetching dependencies.
Changing the project Dockerfile or workload budget requires draining the old host and creating a new host workspace.
The bootstrap refuses surviving containers or a missing preparation record alongside existing private state.
It never clears execution records to make the workspace start.

Before stopping, updating, or deleting the workspace, follow [Drain, update, and retain](#drain-update-and-retain).
Deleting the workspace also deletes both volumes, including retained output and dispatch guards.
Don't enable mandatory restarts or automatic dormancy cleanup for this service template.

### Workload counts and command history

The host saves the normalized request before starting each admitted workload: its ID, command, explicit environment values, template ID, CPU count, RAM in bytes, organization, user, and prepared generation.
Its private state volume also retains timestamps and the observed outcome.
The **total admitted** counter counts these unique records, not HTTP requests.
Retries with the same workload ID don't increment it, and capacity rejections don't count as provisioned workloads.
A canceled workload still counts; an uncertain execution remains recorded rather than being retried.

The workspace's live agent metadata displays **Total admitted workloads** and **Running workloads**, refreshed every ten seconds.
It calls `GET /api/v2/workspaceagents/me/workload-stats` using the current agent token.
This endpoint returns only that registered agent's counts, never commands, and needs no administrator credential in the workspace.
Until registration succeeds, metadata reports unavailable rather than displaying a false zero.

Organization administrators can inspect counts and paginated command history:

```sh
curl --fail --silent --show-error \
  --header @<(printf 'Coder-Session-Token: %s\n' "$CODER_SESSION_TOKEN") \
  "$CODER_URL/api/v2/organizations/$ORG_ID/workload-hosts/$AGENT_ID/stats"

curl --fail --silent --show-error \
  --header @<(printf 'Coder-Session-Token: %s\n' "$CODER_SESSION_TOKEN") \
  "$CODER_URL/api/v2/organizations/$ORG_ID/workload-hosts/$AGENT_ID/history?offset=0&limit=10"
```

Use Bash for these commands.
The history response contains `items`, `total`, and `next_offset`; request the next page until `next_offset` reaches `total`.
Each item includes `request` with the exact command and normalized parameters, plus `workload` with the observed outcome.
Commands can contain secrets. Keep host access and history API access restricted; commands aren't shown in metadata or routine run responses.

Counts and history are **per host**, survive clean host restarts, and remain after a server restart once the host is re-registered.
They aren't a deployment-wide or template-wide lifetime counter.
Replacing or deleting a host doesn't transfer its history to the new host; deleting the workspace deletes its volumes.
The existing limit of 1,024 retained workloads per host still applies.
Read failures surface as errors, not empty history or zero counts.

### Validate the template

Run the checks without a Coder deployment:

```sh
terraform -chdir=examples/workloads validate
bats examples/workloads/tests/prepare-host.bats
shellcheck examples/workloads/host/*.sh examples/workloads/tests/*.sh
```

For an opt-in bootstrap test against a local Linux Docker daemon:

```sh
CODER_WORKLOAD_HOST_DOCKER_TEST=1 bash examples/workloads/tests/smoke.sh
```

The test creates its own privileged host and removes only its own containers, volumes, and image tag.
It verifies image preparation, the project's tests, cgroup limits, clean restart, and refusal to start with a surviving container.
It uses a substitute agent init script; it doesn't enroll a host with Coder.

To test the Terraform resources with the real Docker provider, use Terraform 1.7 or later:

```sh
CODER_WORKLOAD_HOST_DOCKER_TEST=1 bash examples/workloads/tests/provision.sh
```

This test checks that Terraform builds and starts the host image, rather than the project image.
It substitutes the Coder provider and agent init script, then removes its test resources.
It doesn't verify a real agent connection; create a workspace through Coder to check that path.

## Interactive commands

Use `-i` to forward stdin and `-t` to allocate a terminal with automatic resizing. Both stdout and stderr are combined by the terminal. Non-terminal runs retain separate streams.

The prepared image includes Claude Code. To use its interactive interface through `coder run`:

```sh
./scripts/coder-dev.sh run -it --template workload-host \
  --cpu 2 --ram 4GiB --cmd claude
```

Follow Claude's login prompts.
Open the displayed sign-in URL in your browser and paste the returned code into the terminal when prompted.
The prepared image contains no credentials, and no existing host credentials are forwarded.
Login data and filesystem changes belong to this workload's private writable layer and are discarded when it exits.
Do not put API keys into `--cmd`, because command history retains that text.

For terminal animation, a real terminal also gives nyancat its dimensions and normal line buffering:

```sh
./scripts/coder-dev.sh run -it --template workload-host \
  --cpu 1 --ram 100MiB --cmd 'nyancat -f 100'
```

In terminal mode, Ctrl+C is delivered to the application, so Claude can interrupt a response without destroying the session. An external SIGINT to the CLI still requests workload cancellation. Losing the input connection detaches the CLI without replaying keystrokes or relaunching the command.

## Prepare a host manually

If you don't use the sample template, you can configure an existing dedicated service workspace instead.
Use the server, CLI, and agent built from this branch.
Enable `workloads` explicitly on the Coder server, for example:

```sh
./scripts/develop.sh -- --experiments workloads
```

On a **dedicated Linux service workspace**, install Docker with its `overlay2` storage driver, cgroup v2, CPU/memory/swap/PID controls, and the default seccomp profile. The agent's operating-system user needs access to this daemon. That is administrative access to the host, so only trusted operators may administer the workspace. A Docker socket pointing at another machine or at a shared Kubernetes node is not a supported host configuration.

The workspace's allocation must cover the configured allocatable budget, the agent/supervisor, Docker, page cache, and preparation. Configure the budget from the outer workspace allocation, not from physical node totals. Do not run unrelated containers against the daemon. For Kubernetes or nested Docker, controller delegation and the daemon's placement inside the outer allocation are operator prerequisites, not something this POC provisions automatically.

Build the demo image **before registering capacity**:

```sh
docker build -t coder-workload-demo:coder examples/workloads
```

This checks out Coder commit `e26c1934a5f96c5209e7b054419db5f6c5c12323`, prepares its development dependencies, and runs representative unit tests. A preparation failure does not publish a new image. No fetch or dependency installation happens on `coder run`.

Create a private configuration file on the workspace. The example budget below requires more than 4 CPUs and 8 GiB of outer capacity once host and preparation headroom are included. Reduce the budget to fit your actual allocation.

```sh
umask 077
jq -n --arg token "$(openssl rand -hex 32)" '{
  token: $token,
  state_dir: "/var/lib/coder-workloads",
  image: "coder-workload-demo:coder",
  workdir: "/work/coder",
  shell: "/bin/sh",
  repositories: {
    "/work/coder": "e26c1934a5f96c5209e7b054419db5f6c5c12323"
  },
  cpu: 4,
  memory_bytes: (8 * 1024 * 1024 * 1024)
}' > workload-host.json
```

Use a persistent state directory writable only by the agent user. Set `CODER_AGENT_WORKLOAD_CONFIG` to the absolute path of this file in the service workspace's agent environment, then start/restart that agent. Initialization fails explicitly when the profile is unavailable. The image and configuration must be ready before agent startup; ordinary agent startup scripts run later.

The dedicated token is for supervisor control. It is **not** a Coder session token or `CODER_AGENT_TOKEN`. Do not bake any of these credentials into the prepared image.

## Register and run

As an organization administrator, enroll the workspace agent. Set `CODER_URL`, `CODER_SESSION_TOKEN`, `ORG_ID`, and `AGENT_ID` for your deployment. Run this where the private host configuration is available:

```sh
jq --arg agent "$AGENT_ID" '{agent_id: $agent, token: .token}' workload-host.json |
  curl --fail-with-body --silent --show-error \
    -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
    -H 'Content-Type: application/json' \
    --data-binary @- \
    "$CODER_URL/api/v2/organizations/$ORG_ID/workload-hosts"
```

Registration binds the host to its actual workspace organization, template, and current template version. Selecting that template does not instantiate a new workspace. Register only service hosts in the location where that organization's source and data are permitted to reside.

Then, as a user with template-use and workspace-create permissions:

```sh
coder run --org "$ORG_ID" --template platform-development \
  --cpu 2 --ram 4GiB --cmd 'go test -count=1 ./coderd/util/ptr ./coderd/util/slice'

coder run --org "$ORG_ID" --template platform-development \
  --cpu 1 --ram 512MiB \
  --cmd 'printf private > private-marker; cat private-marker'

coder run --org "$ORG_ID" --template platform-development \
  --cpu 1 --ram 512MiB --cmd 'test ! -e private-marker'
```

Repeat `--env KEY=value` to supply environment variables:

```sh
coder run --org "$ORG_ID" --template platform-development \
  --cpu 1 --ram 512MiB --env GREETING=hello --env MODE=demo \
  --cmd 'printf "%s %s\n" "$GREETING" "$MODE"'
```

This prints `hello demo`.
Empty values are allowed, duplicate keys use the last value, and commas or newlines stay literal.
`--env KEY` is rejected rather than inherited from your shell.
Environment values aren't printed in routine diagnostics, but administrator-only run history retains them in plaintext.

Template names and IDs are accepted; ambiguous names need `--org`. By default, only command stdout and stderr are printed, and normal completion propagates the command's exit status. Add `-v` or `--verbose` to print the attached host workspace, workload identity, prepared generation, and other diagnostics to stderr. If the host workspace is not visible to your user, verbose output identifies its agent instead.

Verbose runs also print a timing summary and the exit code after draining output:

- **Provision:** Creating the isolated container from the prepared image. This excludes image preparation, cloning, and dependency installation.
- **Exec:** Docker's process start-to-exit duration, excluding output draining and cleanup.
- **Overhead:** Total minus provision and exec. This is all elapsed time outside those two host intervals, including dispatch, container attachment/startup, and output collection where they do not overlap execution or provisioning.
- **Total:** CLI handler entry through the final drained output frame. This includes stream setup, output delivery, and verbose workspace lookup, but excludes CLI process startup and flag parsing.

Provision, exec, and overhead are non-overlapping parts of total. Their displayed percentages are rounded together to sum to 100.0%. Total shows only a duration. If host measurements are missing, negative, or exceed total, or total is zero, the breakdown is marked unavailable and percentages are omitted rather than inventing a split.

Two latency readings are shown separately, without percentages:

- **Dispatch:** CLI handler entry through the admission response, including template lookup, authorization, and host selection.
- **First output:** CLI handler entry to the first nonempty stdout or stderr received, or `none` for silent commands.

These latencies can overlap provisioning and execution; they are not additional phases.

Output uses one continuous HTTP stream from the supervisor through the server to the CLI, not repeated polling. The supervisor sends output as it arrives and blank heartbeat lines while idle. The CLI reconnects only if that stream breaks, resuming from the last delivered event offset without relaunching the command. Reconnect attempts without new output are bounded, and HTTP throttling respects `Retry-After`.

Without `--tty`, Ctrl+C requests cancellation independently of the stream. With `--tty`, Ctrl+C goes to the application; an external SIGINT to the CLI requests cancellation. A network disconnection only detaches observation. Within one server lifetime, a retry of the same API workload ID cannot relaunch the command; a **new CLI invocation is a new workload**, not reattachment. The original host also retains its dispatch guards across restarts, but there is no cross-host recovery protocol after the server loses its placement index. Do not resubmit an uncertain command after a server restart or host replacement. The API offers retained output by event offset; add `follow=true` to receive `application/x-ndjson` frames on the same connection. A terminal frame with no events confirms that output has been drained. Snapshot requests without `follow=true` remain available. There is no separate workload-management CLI in this prototype.

No eligible prepared capacity produces an explicit error. There is no queue, Terraform fallback, implicit overcommit, or automatic retry of an uncertain execution.

## Drain, update, and retain

Before a planned workspace stop, template update, or image change, drain it:

```sh
curl --fail-with-body --silent --show-error --request DELETE \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/organizations/$ORG_ID/workload-hosts/$AGENT_ID"
```

Draining rejects new placements; it does not cancel existing commands. Wait for active workloads and cleanup to finish before stopping the workspace. Disable ordinary autostop and mandatory restart policies for demo hosts: workload activity is not yet integrated into those policies.

Generation publication is manual in this POC. Build and validate a new image, drain the old host, wait for completion, and update the private configuration. This POC cannot re-enroll a drained agent in the same server process: restart the agent and Coder server, then register again. Restarting the server discards its placement index, so first finish all workloads on every registered host. A running workload never follows a mutable image tag. Do not prune images referenced by active containers. There is no automatic upstream fetch, refresh loop, or multi-host generation publication protocol.

Private filesystem changes are discarded after execution. Each run retains at most 4 MiB or 4,096 output events, whichever limit is reached first. Interactive runs retain the most recent output and keep streaming after reaching this limit; ordinary runs retain the initial output. A slow or reconnecting reader can miss output that has left the retained window. Verbose mode warns when retained output is truncated. Output is persisted on completion, not continuously, so an interrupted supervisor can lose its latest output. API reads return batches of at most 256 KiB or 128 events. The supervisor retains at most 1,024 workload IDs without eviction. There is no artifact extraction or automatic merge into the canonical repository.

Keep the private state directory when restarting the agent. It contains command text and output in owner-only files; trusted operators and Docker administrators can inspect commands. Startup refuses active or unknown records, pending cleanup, missing completed output, or any surviving Docker container. There is no automatic recovery after an interrupted active run. Do not delete uncertain execution records to make capacity available: inspect and fence the original containers first. A clean restart reopens a drained supervisor only after these checks pass; the Coder server must also restart before re-enrolling the same drained agent.

## Measure the experience

For an enrolled host, run:

```sh
python3 examples/workloads/benchmark.py \
  --coder /path/to/this/branch/coder \
  --org "$ORG_ID" --template platform-development \
  --cpu 1 --ram 512MiB --samples 100 --concurrency 1
```

The benchmark launches a **new CLI process per sample**. Its marker follows checking that a Coder source file is present and invoking an installed frontend dependency. This checks prepared-environment access, not completion of Coder's test suite. It measures local monotonic elapsed time through observation of that marker; it does not subtract clocks on different machines. It reports p50/p95/p99 and failures separately. Record network distance, cache conditions, host allocation, and other load alongside results. Increase concurrency only within reservable capacity.

An opt-in Go integration test creates a test Coder deployment and real agent transport using the prepared Docker image:

```sh
CODER_WORKLOAD_DOCKER_TEST=1 go test ./cli -run '^TestRunDocker$' -count=1 -v
```

The test requires a dedicated empty daemon, not a developer's shared Docker daemon. Set `CODER_WORKLOAD_TEST_DOCKER_HOST=unix:///path/to/dedicated/docker.sock` to select one. The prepared image must be loaded into that daemon. The supervisor's equivalent configuration field is `docker_host`; it does not read `DOCKER_HOST`.

Set `CODER_WORKLOAD_TEST_BINARY` to an absolute branch binary path and `CODER_WORKLOAD_BENCHMARK_SAMPLES=100` to include actual process startup in that test's latency samples. Without the binary setting, it runs the CLI in-process and is not a complete invocation benchmark. The test also checks private filesystem isolation, stdout/stderr and exit codes, cgroup settings, memory-limit termination, capacity rejection, idempotency, cancellation, and draining.

### Earlier prototype verification results

These results used the previous ItsDangerous image, not the Coder development image. They aren't Coder project latency measurements.

On 2026-11-04, 100 fresh CLI processes completed against a local test Coder server and agent, with one invocation at a time. The dedicated nested Docker 27.5.1 daemon had 4 CPUs and 4 GiB, used `overlay2` and cgroup v2, and served a configured 2-CPU/2-GiB workload budget. Each command requested 1 CPU and 512 MiB. The prepared image was `sha256:7aead6ca0a3da2a3c1bde63df3ebe9aafd4daa93105e37410d1b903df6c363cb`. Source and dependencies were already prepared; no cache flush or warm-up samples were excluded.

Two 100-process runs completed with zero failed samples:

| Conditions                               | p50    | p95      | p99      | Maximum  |
|------------------------------------------|--------|----------|----------|----------|
| Other development checks running         | 316 ms | 1,194 ms | 1,589 ms | 2,355 ms |
| Final run after development checks ended | 312 ms | 316 ms   | 363 ms   | 367 ms   |

All 100 final-run samples reached useful project work within one second. The earlier loaded run exceeded one second at the tail, so this is not a subsecond latency guarantee under contention. These local results include CLI startup, authorization, placement, container startup, real project work, and observation of its output marker; they do not predict remote-network or hostile-tenant performance.

## Deliberate limitations

- One Coder server process and one active host per template. Host registration and placement indexes are in memory; they must be recreated after server restart. The server retains at most 1,024 workload IDs and never evicts them to admit more work. Host dispatch guards are separate persistent state. Do not retry uncertain commands with new IDs.
- Docker containers share the host kernel. This is not a microVM-grade boundary or a production hostile-tenant service.
- Private writable layers currently have **no byte or inode quota**. A disk-filling workload can disrupt its host. Run trusted, bounded demos only and monitor host storage. CPU, memory, and process enforcement do not fix disk exhaustion.
- Host capacity and location are explicitly operator-configured. The POC does not prove nested controller ancestry, reserve exclusive CPUs, or implement additional location policy beyond the registered organization/template pool.
- No automatic refresh, autoscaling, scale-to-zero, checkpointing, cross-host migration, transparent retry, or presets.
- No artifact extraction, built-in firewall policy, persistent interactive sessions, or externally issued workload credentials.
- Workspace stop/update/delete does not automatically drain first. An operator must drain; a forced host stop can leave an unknown outcome.
- No production audit integration or output browser. Command bodies and supervisor credentials are excluded from workload SDK body logging, including verbose body logging.

The subsecond goal must be measured for the supported prepared-capacity environment. Passing an API or isolation test alone does not establish that latency target.
