# Native sandbox experiment validation

This is an experimental single-host provisioner, disabled by default. It uses
normal Coder workspace identities and provisioner jobs. Each start creates a
fresh container, writable filesystem, network namespace, and agent credential.
The host image is cached and unpacked; sandbox instances are not precreated.

## Real runtime measurement

On September 14, 2026, a private patched Coder server on an Azure Standard_B4ms
host completed 100 sequential 1-vCPU / 2-GiB sandbox starts using containerd
2.2.8 and gVisor release-20260907.0 with systrap. The host had four burstable
vCPUs, 16 GiB RAM, four workers, and a cached image. The shared outer Coder
server managed the VM; the private server managed the native sandboxes.

The timer starts before the workspace-create request and ends when authenticated
SSH `true` succeeds through Coder. It includes queueing, provisioning, job
completion, agent connection, and command execution. Image preparation,
repository preparation, connection teardown, and deletion are excluded. Each
sandbox was deleted before the next sample. The client ran on the host.

| Duration | p50 | p95 | p99 |
| --- | ---: | ---: | ---: |
| Request to successful command | 1.008 s | 1.630 s | 5.959 s |
| Queue | 0.012 s | 0.024 s | 0.029 s |
| Provisioner job | 0.309 s | 0.339 s | 0.349 s |
| Runtime creation | 0.205 s | 0.238 s | 0.248 s |

All 100 requests completed with zero readiness, timing, or API-cleanup failures.
Four took more than five seconds; the maximum was 6.080 seconds. Those agents
were observed connected within 0.637 seconds; most remaining time was in the
connection-and-command stage. Every sample recorded one connection attempt.
The cause of the long tail within that stage is unresolved.

[Sanitized samples](sequential-starts.json) include the original implementation
base, patch hash, image digest, and all timing values, excluding deployment,
workspace, and build identifiers. Percentiles use nearest rank and were
independently recomputed. Stage durations overlap and must not be added;
observed milestones include 20-ms polling delay. These results do not establish
four-way latency, sustained throughput, remote-client latency, or performance
without CPU burst credits.

The measured implementation used base commit
`25af21b5d5d86ab7d9253e1e72d33b2da170e4f8`. The PR subsequently rebases the
integration onto current main, resolves the module-cache guard, and renumbers
the database migration. The measurements are evidence for that prototype, not
a new benchmark of every subsequent PR revision.

## Functional checks

The real Linux/amd64 host passed native YAML-only import and dry-run with
Terraform absent, create, authenticated SSH, terminal allocation, stop, clean
restart, fresh agent credentials, normal deletion, repeated deletion, and
cleanup after missing Coder state. Git, Go, C, Node, and Python smoke workloads
passed. Reconnecting-terminal authentication, interactive input, resize to
40 by 100, and reconnection to the same shell passed with the final PTY guard.
The inner server was headless; frontend rendering was not tested.

A server crash captured during container creation preserved durable intent and
uncertain allocations. An existing workspace remained accessible after the
worker restarted. The uncertain creation could not be deleted on the same
host boot. After an operator stopped and restarted the host, normal API
deletion succeeded. This is a known manual recovery requirement, not automatic
worker or host recovery.

After the final 100-start run, independent inventory checks found no remaining
inner workspaces, containerd containers or tasks, direct runsc allocations,
writable sandbox snapshots, native network directories, or CNI IP allocations.
The server, containerd, and database remained active. Committed image layers
and deleted-workspace operation history were intentionally retained.

## Remaining validation

- Four-request bursts require a host with enough non-oversubscribed capacity.
  The available quota only allowed the four-vCPU burstable host.
- The public [Azure host recipe](azure-host/README.md) replaces the private
  patch-delivery mechanism with a pinned public source commit. Its Terraform
  and shell checks are separate from the earlier deployed recipe; the public
  variant needs a fresh end-to-end deployment before promotion.
- Persistent user disks, snapshots, Docker-in-Docker, arbitrary user images,
  multiple hosts, and automatic failure recovery remain outside this prototype.

The original implementation passed native and benchmark unit/race tests,
CLI/API integration tests, generation, TypeScript checking, Linux builds, and
Terraform regression tests. See the draft PR and CI for checks on the rebased
revision. Human review is required before promoting this experiment.
