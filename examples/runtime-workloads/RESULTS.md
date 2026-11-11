# Results

Date: 2026-11-11. Cluster: single-node k3s v1.36.5 with Cilium, in a QEMU VM
under software emulation. coderd: built from this branch, one replica,
outside the cluster.

## Outcome

Every check in [validate.py](validate.py) passed.

| Run | Result   | Notes                                                                                                                                                  |
|-----|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1   | 47 of 49 | Both failures were bugs in the validation script, not the system. This run included the ten-minute token expiry and rotation check (D6), which passed. |
| 2   | 48 of 48 | Fixed script. D6 was skipped, since it is opt-in (`--rotation`).                                                                                       |
| 3   | 48 of 48 | The template, operator manifest and script exactly as committed in this directory. D6 skipped.                                                         |

Token handling did not change after run 1, so D6 was not repeated.

Go tests on the branch pass: `coderd/x/workloads` (token verification and the
registry), `cli/workloadoperator` (pod cloning and the reconcile plan), and
`coderd` `TestRuntimeWorkloads` and `TestRuntimeWorkloadsDisabled` (the whole
flow against a test coderd, with a test standing in for the cluster and the
operator).

## What the checks show

**A. Existing behaviour is unchanged (8 checks).** A workspace from an
ordinary single-daemon template was built before the operator existed and is
connected and healthy. `coder run` on it is refused with a clear message. The
human's daemon keeps its agent token.

**B. Runtime provisioning (13 checks).** `coder run dev -- coder identity`
creates a pod and prints an identity of the form
`<workspace UUID>/pod/<pod UID>`. Two workloads and the human's daemon have
three different identities, all associated with the workspace owner. No
workspace build runs. The pod is a clone of the shape, holds no Coder
credential, and no Secret is created.

**C. Masquerade attempts (13 checks).** All refused:

- alpha's token with beta's IDs in headers and query: still alpha.
- alpha's token rewritten to name beta's pod.
- alpha's exact claims signed by another key.
- A service account token not bound to a pod.
- A token bound to alpha's pod for the Kubernetes API audience.
- A pod the operator did not create, with the same service account and audience.
- A foreign pod carrying beta's labels: no identity, deleted by the operator, beta unaffected.
- A foreign pod created first with a pending workload's labels.
- A pod token or the human daemon's agent token calling `coder run` or a user route.
- The workload service account reading pods, Secrets or Deployments, or minting tokens.

**D. Revocation and lifecycle (up to 10 checks).** Removing a workload
revokes its identity at once, with 553 seconds left on the copied token.
Deleting a pod behind Coder's back removes the workload, and it is not
recreated. A pod token expires after ten minutes and the daemon carries on
with the rotated one. Stopping the workspace removes every runtime daemon;
starting it restores only what the template declares.

**E. Operator scope (2 checks).** The operator can list and create pods and
read Deployments in its namespace. It cannot read Secrets, exec into pods, or
see other namespaces. Nothing in the namespace is bound to a cluster role.

## Limits

- **A copied pod token works from elsewhere** until it expires (at most ten
  minutes) or its pod is removed (check D1). coderd sees every pod behind one
  NAT address here, so it cannot bind a token to its source.
- **Any process in a workload pod shares that pod's identity**, as with any
  workspace daemon.
- **Permissions are unchanged.** A runtime daemon is authorized like any
  agent of the workspace: with the owner's permissions. That includes the
  owner's Git credentials, which was deferred.
- **The operator and the namespace's administrators are trusted.** The
  operator can vouch for any pod in its namespace. Anyone who can mint
  service account tokens or exec into pods there can act as a workload.
- **The operator logs in to coderd with an admin API key.** A dedicated
  enrolment credential does not exist yet.
- **If the operator stops syncing for 30 seconds, coderd stops accepting its
  pods' tokens** until it returns. Connected daemons stay connected.
- **Operator and pending-request state is in coderd's memory.** It needs a
  single coderd replica. Bound workloads survive a coderd restart; pending
  requests do not.
- **The shape's agent still exists in the API**, where it shows as timed out.
  The web UI hides it and workspace health ignores it.
- **`coder run` streams output only.** There is no stdin or TTY yet; a
  terminal is available through `coder exp rpty <workspace>.<workload>`.
- **Not built:** autostop extension, graceful stop, volume clones, egress
  attribution, Terraform provider changes, audit log entries. The web UI
  only lists workloads.
- **The template uses a pinned alpine image** with a mounted `/etc/passwd`,
  because that image was already on the test node. The agent needs
  `/etc/os-release` and a passwd entry for its user.
