// Package agentworkload provides an experimental, local Docker workload
// supervisor. It is not a hostile-code sandbox or a general Docker scheduler.
//
// The supported profile is Linux, Docker overlay2, cgroup v2, and the default
// seccomp profile. The daemon must be dedicated to the outer workspace, with
// exactly one supervisor state directory and no other Docker writers. Preparation
// (including pulling/building images, repositories, and dependencies) happens
// before enrollment. Repository commits are operator attestations, not checked
// by executing commands at launch. Generation IDs are resolved Docker image IDs.
// Images must not declare VOLUMEs or contain credentials. Image environment is
// trusted; host environment, credentials, sockets, and directories are not passed
// into containers. Each run gets a private writable image layer, Docker bridge
// networking, no capabilities, no-new-privileges, 256 PIDs, whole CPU quotas,
// and a memory limit with memory+swap equal to memory (no swap).
//
// Disk layer quotas are UNSUPPORTED. A workload can fill the outer workspace's
// disk. Docker json-file logs rotate at 2 MiB with two files; the supervisor
// retains at most 4 MiB and 4096 output events per workload, truncating the suffix.
// A Docker log record can exceed the rotation threshold. GET returns batches of
// at most 256 KiB and 128 events with stable event offsets. Output is persisted
// on completion, not on every write. State retains up to 1024 idempotency keys;
// reaching that limit rejects new work and never evicts an old key.
//
// Resource admission reserves requested memory plus 64 MiB per run for runtime
// overhead. Configured capacity is allocatable capacity, after subtracting outer
// agent/daemon overhead. Daemon totals and visible local cgroup ancestor limits
// are checked, but the operator must ensure the daemon and supervisor share the
// intended outer allocation and leave adequate fixed overhead.
//
// Close detaches without killing running commands. Recovery with any outstanding
// reservation, incomplete cleanup, or surviving Docker container fails closed.
// An operator must reconcile containers named coder-workload-<UUID> and the
// private state before re-enrollment. Never delete state or resubmit a command
// with a new ID to resolve an uncertain outcome. Automatic replay and active-run
// recovery are deliberately unsupported in this POC. State must remain on a
// durable local filesystem, private to the supervisor, and must not be edited
// while it is running. Docker errors and commands are omitted from diagnostics.
package agentworkload
