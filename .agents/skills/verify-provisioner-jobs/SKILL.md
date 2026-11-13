---
name: verify-provisioner-jobs
description: "Model-checks the provisioner job lifecycle (acquire, heartbeat, CompleteJob, FailJob, the hung-job reaper, user cancel, prebuild pending cancel) with TLA+ over every interleaving of two daemons and two builds of one workspace. Use when a change touches provisioner job SQL, coderd/provisionerdserver, coderd/jobreaper, build cancel, wsbuilder's active-build check, the prebuild pending-job cancel, or the provisionerd runner's heartbeat and completion, or when asked whether job outcomes can be lost, duplicated or rewritten."
---

# Verify provisioner jobs

`ProvisionerJobs.tla` models the provisioner job row of two consecutive builds of one workspace (build N is job 1, build N+1 is job 2). TLC checks every order of these steps:

- Two daemons acquire, heartbeat, freeze, crash, and send `CompleteJob` or `FailJob`.
- The reaper scans for stale jobs and then reaps them.
- A user cancels a build.
- The prebuilds reconciler cancels a pending job.
- A user starts the next build.

Each action is one Postgres statement, or one transaction that holds the job row lock. Steps the code runs as separate statements without a lock are separate actions. The comments in the spec name the code behind each action.

## When to use

Run it when your change touches a file in `coverage.txt`, or changes how a job row is read or written. `run.sh` lists the covered files that changed since the last sync. If your change touches none of them and no new caller writes `provisioner_jobs`, the check does not apply.

## Run

```sh
.agents/skills/verify-provisioner-jobs/run.sh
```

It needs Java 11 or newer: `java` on `PATH`, `JAVA=<path>`, or `mise` (it then runs `mise exec java@temurin-25`). The first run downloads `tla2tools.jar` 1.7.4 into `build/verify-provisioner-jobs/` and checks its SHA-256. A run takes about 15 seconds. The TLC logs stay in a temp folder only when a check fails. Set `OUT=<dir>` to keep them in a folder you choose.

The script prints one line per config and ends with `checks: N ok, M failed, 10 total`. It exits 0 only when every config gives its expected result. A run without the count line did not finish.

A pass checks the spec, not your diff. When a covered file changed since the last sync, `run.sh` lists it and prints a `WARNING` line. Until you update the spec to match the change (see "When your change is intended"), a pass tells you nothing about it.

Read every log with the `/statechart` skill from the pstack plugin. It turns each violation into the shortest path of steps in plain words and draws the state graph of a passing config. Without it, read the trace in `$OUT/<config>.out`: TLC prints each state with the action that reached it.

## What it checks

The code keeps these promises. `Safety` checks all of them and must pass.

| Property | The promise | Harm if broken | Code that keeps it | Config that breaks it |
|---|---|---|---|---|
| `OneRunnerPerJob` | At most one daemon runs a job. | Two Terraform runs on one workspace duplicate or corrupt infrastructure. | `AcquireProvisionerJob` picks the row `FOR UPDATE SKIP LOCKED` inside the `UPDATE` (`coderd/database/queries/provisionerjobs.sql`). | `NegAcquireNoLock` |
| `CancelNeverClosesRunningJob` | A cancel never sets `completed_at` while a daemon holds the job. | The next build starts while Terraform still runs. | `patchCancelWorkspaceBuild` sets `completed_at` only without a worker (`coderd/workspacebuilds.go`). The prebuild cancel runs at Repeatable Read (`enterprise/coderd/prebuilds/reconcile.go`). | `NegCancelIgnoresWorker`, `NegPrebuildCancelReadCommitted` |
| `ReaperSparesLiveJob` | The reaper fails only jobs whose `updated_at` is stale. | A healthy build is failed and the next one runs over it. | `reapJob` rechecks `updated_at` under the row lock (`coderd/jobreaper/detector.go`). | `NegReaperNoStaleRecheck` |
| `LatestStateTracksApply` | When an apply finished and its report arrived, the workspace's latest build holds the state Terraform wrote. | The next build plans against old state, so it duplicates or orphans cloud resources. | `CompleteJob` writes the state even after a reap. `FailJob` saves the state of a completed job while its build is the latest (`saveStateOfCompletedJob` in `coderd/provisionerdserver/provisionerdserver.go`). | `NegLateFailDropsState`, `NegLateWriteRejected` |

Two more negative controls guard the late-write rules:

- `NegReaperNoCompletedRecheck` removes the reaper's `completed_at` recheck. It sets `LateWrite = "state"` so that only the reaper can change an outcome, and `OutcomeIsFinal` must break.
- `NegLateWriteRejected` makes `CompleteJob` and `FailJob` write nothing for a completed job. It shows why "guard every write with `completed_at IS NULL`" is the wrong fix for a late report: the latest build keeps the state of the build before it.

Each `Neg*` config turns off one guard that the code has. It must break its property. If it passes, the property no longer depends on that guard, and the model is wrong.

## What the code does not promise

Two configs check behavior that the code allows on purpose. `run.sh` expects TLC to report each property broken. If one passes, the code changed, so update this section.

- `NotPromisedOutcomeIsFinal`: a daemon that resumes after the reaper failed its job can still send `CompleteJob`, which marks the build succeeded. This is intended: the late apply created the resources, and the workspace then works without a restart.
- `NotPromisedNextBuildSeesApply`: the reaper unblocks the next build while a paused daemon still holds the old one. If a user starts build N+1 before the late report arrives, build N+1 starts from the state of build N-1. No write rule fixes this, because the apply finishes after build N+1 copied its state. A fix must keep build N+1 from starting while the old daemon can still apply.

The heartbeat after a reap also tells the daemon to continue (`UpdateJob` reads only `canceled_at`). The model has no step for a heartbeat that cancels.

## A real bug or a modeling error

When a config gives a result its row in `checks` does not expect, before or after you edit the spec:

1. Read the trace with `/statechart`. Map each step to the code in the action's comment.
2. Replay the path against the real code as a Go test. `TestFailJobAfterReap` in `coderd/provisionerdserver/provisionerdserver_test.go` is the pattern. It acquires a job, sets `updated_at` in the past to stand in for time passing, runs `jobreaper.New` with a tick channel, and then sends the late RPC. To put a step between the reaper's scan and its locked transaction, wrap the `database.Store` and run the step when `GetProvisionerJobByIDForUpdate` is called.
3. If the real code does what the trace says, it is a finding. Report the path in the code's words. Do not weaken the property or the model to make it pass.
4. If the real code does not do it, the model is wrong. Find the action whose guard or atomicity differs from the code and fix it.

## When your change is intended

1. Find the actions that your change touches, using the code names in the spec comments. A new guard either matches an existing switch, or it needs a new action or variable.
2. Edit the spec so that it says what the new code does. Keep each action as large as one statement or one locked transaction in the code.
3. Run `run.sh`. The `Safety` config must still pass, and each `Neg*` and `NotPromised*` config must still break its property. If one does not, follow "A real bug or a modeling error".
4. When you add a guard, add a switch for it and a `Neg*` config that turns it off. Add the config's row to `checks` in `run.sh`. When you remove part of a guard, give each part its own switch first, as `ReaperRechecksCompleted` and `ReaperRechecksStale` do.
5. When `run.sh` lists covered files that changed, read their diffs against the model. After you confirm the model describes them, run `run.sh --sync`. It records the git blob hash of each covered file in `coverage.txt`. Hashes survive squash merges, where a recorded commit would not.

## Assumptions and limits

- Bounds: two daemons, two builds of one workspace, and every interleaving of their steps. `Safety` explores about 47,000 distinct states.
- Time is one flag per job: `updated_at` is older than the reaper threshold. A job goes stale only when no live process holds it. That means the job is pending, its daemon crashed, or its daemon is frozen. A live runner heartbeats every few seconds, and after one failed 30 second update it stops and sends `FailJob` (`provisionerd/runner`). A frozen daemon stands for a paused process or VM, or for replica clocks skewed past five minutes. The model does not check whether real clock skew reaches that.
- A daemon crashes only before its apply ran. A crash after an apply loses the state with the process, and no server change can recover it.
- A `FailJob` carries state only after an apply ran. The `FailJob` after a failed heartbeat or a cancel timeout carries none (`failedJobf`).
- Postgres: the default isolation is Read Committed. A row lock serializes the writers of a row. `SKIP LOCKED` skips a row that another transaction holds. Repeatable Read fails a statement whose row changed after its snapshot.
- The model leaves out these parts:
  - liveness (whether every job finishes)
  - pubsub delivery
  - logs, quota, resources and agents
  - provisioner tags and organizations
  - the acquirer's in-memory queue
  - template import and dry-run jobs
  - more than two builds or daemons
