# Prototype evidence: October 28, 2026

**Result: blocked before end-to-end validation.** This branch contains a real
Terraform template and live validation harness, not a mockup. The template
provisioned a KubeVirt VM and imported its persistent disk. An Ubuntu guest
reached its login banner. No Coder agent connection or Docker workload result is
being claimed.

## Observed results

| Check                                                                                  | Result             | Evidence and limit                                                                                                                                                                                                |
|----------------------------------------------------------------------------------------|--------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Isolated Kubernetes cluster                                                            | Passed             | kind v0.33.0, Kubernetes v1.36.4; KubeVirt v1.9.1 and CDI v1.66.1 reported Deployed.                                                                                                                              |
| Namespace-scoped provisioning                                                          | Passed             | Workload writes used the dedicated ServiceAccount. Cross-namespace Secret access was denied. Read-only CRD discovery was also required.                                                                           |
| Image import                                                                           | Passed             | CDI reported Succeeded for the checksum-pinned Ubuntu image and the root PVC was Bound.                                                                                                                           |
| Real Coder template import and Terraform creation                                      | Passed after fixes | Local Coder imported template version `prototype-v3` and Terraform created the VM without an inconsistent-state error.                                                                                            |
| VM boot                                                                                | Partial            | KubeVirt reported Running/Ready. The nested-KVM attempt reached the Ubuntu 24.04.5 login banner, but the runner then panicked.                                                                                    |
| Coder agent                                                                            | Not connected      | The final recorded agent status was `connecting`, lifecycle `created`, with `first_connected_at: null`.                                                                                                           |
| Docker build/run, bind mounts, generated files, Compose, and Docker restart continuity | Not run            | The harness never got through its real-agent readiness gate.                                                                                                                                                      |
| Workspace-file and Docker-volume persistence across stop/start                         | Not run            | No successful workload preparation or verification phase exists.                                                                                                                                                  |
| Fresh guest credentials and guest permissions across stop/start                        | Not run            | The code and checks exist, but no connected guest completed them.                                                                                                                                                 |
| Devcontainers, kind inside the workspace, production isolation, and performance        | Not tested         | Do not infer these from VM readiness or from Docker running on the separate lab runner.                                                                                                                           |
| Workspace resource cleanup                                                             | Passed             | After halting the unstable guest, Coder stop/delete completed; the dedicated namespace contained no VM, VMI, DataVolume, PVC, pod, or Secret. This is not proof of graceful shutdown of a working guest workload. |

## Why the live run stopped

The outer Coder workspace exposed `/dev/kvm`, but opening it returned permission
denied as both the workspace user and local root. A private device-node probe was
also denied. The shared host device's permissions were not changed.

A disposable Ubuntu runner was therefore booted with QEMU TCG, 8 vCPUs and 12 GiB
RAM. KubeVirt's first inner VM also used software emulation. It mounted its root
disk, but systemd failed with:

```text
Failed to fork off sandboxing environment for executing generators: Protocol error
Failed to start up manager.
```

A second attempt used KVM inside that software-emulated runner. A minimal
`KVM_RUN` probe successfully executed a halt instruction, and the KubeVirt guest
then reached its Ubuntu login banner. That was not a reliable substitute for real
hardware virtualization: the runner logged unhandled KVM MSR writes, followed by:

```text
Kernel panic - not syncing: corrupted stack end detected inside scheduler
Rebooting in 10 seconds..
```

The runner reboot broke the Kubernetes tunnel. The last recorded Coder agent had
never connected. The live harness was stopped with `complete: false`; its Docker
and lifecycle assertions did not run. Neither software-emulated attempt is
performance evidence or a successful hardware-KVM qualification.

## Problems found and corrected before handoff

- The Terraform manifest provider needs read-only CRD discovery in addition to
  namespace-scoped workload permissions. The example role now grants only those
  additional discovery verbs.
- `qemu64` was not advertised by the lab node. The test selected the advertised
  `Nehalem` model instead of forging a scheduling label.
- KubeVirt's default guest subnet overlaps the outer QEMU host gateway. The
  template now exposes a separate guest-side masquerade subnet.
- KubeVirt v1.9.1 injects `kubevirt.io/pci-topology-version: v3` into flexible
  template metadata. Declaring that annotation fixed the observed provider
  object-type mismatch; no broad `computed_fields` override was used.
- A Terraform VM-readiness waiter did not complete cancellation promptly, leaving
  one VM outside saved state when the canceled process was killed. That owned VM
  was identified and removed. The template now records the VM promptly; the live
  harness separately requires both KubeVirt readiness and the connected Coder
  agent before running workloads.
- Review found that persistent disks would keep old startup code if it were only
  installed by cloud-init. The per-start Secret now carries the bootstrap code,
  and the agent's service and credentials are rebuilt under `/run`.

The code-level fixes were reviewed independently. The guest bootstrap, token
replacement, permissions, and workload checks still need a completed live run.

## Provenance

The Coder control plane and CLI were built from repository commit
`0b8d9135ae47a7c0b9e096315a306e4e36a39f07`, reporting
`v2.38.3-devel+0b8d9135ae`. The running template version was
`75793ab4-bb95-4644-a932-b9107abdd84c` (`prototype-v3`). Its uploaded archive was
downloaded from that local Coder instance and all seven listed implementation and
workload-fixture files matched the hashes in
[evidence/2026-10-28.json](evidence/2026-10-28.json).

The live runner used Ubuntu kernel `6.8.0-147-generic` and outer QEMU `10.2.1`.
The workspace VM requested 2 vCPUs, 2 GiB RAM, and a 24 GiB root disk. The image
came from Ubuntu `release-20261017` with the SHA-256 recorded in the template
instructions. The root image was verified against Ubuntu's signed checksum file
before the runner was booted, and the workspace import supplied the same checksum
to CDI.

The lifecycle checker was strengthened after the blocked run to reject unexpected
uploaded files. No passing run is attributed to that revision either. The
machine-readable evidence intentionally excludes tokens, private keys,
kubeconfigs, Terraform state, VM disks, and customer/commercial data.

## Local checks

The following completed successfully: Terraform formatting and validation,
ShellCheck, shell formatting, Compose configuration validation, Python parsing,
Markdown lint, evidence JSON parsing, and verification of the seven recorded
source hashes. The lifecycle command also rejected a nonlocal Coder URL before
creating any state. The repository's `make pre-commit-light` checks passed.
These checks do not replace the blocked live workload suite.

## Cleanup

The owned workspace and kind cluster were deleted. The outer QEMU runner shut
down with exit code 0, and the local Coder server and port forwards were stopped.
Generated SSH keys, kubeconfigs, cloud-init data, Terraform state, VM disks, and
the local development database were removed. Installed command-line tools and
ordinary dependency caches remain; no test services were left running.

## Required next validation

Run the unchanged runtime template on a disposable runner with **usable hardware
KVM**, with platform-approved KubeVirt/CDI and storage. Complete all four phases
of `tests/lifecycle.py`, then separately test Docker-backed devcontainers, kind,
performance, upgrades, migration, storage failure, and customer security policies.
Do not work around the emulated runner's kernel failure by disabling runtime or
guest security checks.

Generated by Coder Agents on October 28, 2026.
