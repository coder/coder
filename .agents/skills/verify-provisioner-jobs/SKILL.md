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

Run it when your change touches a path in `coverage.txt`, or changes how a job row is read or written. `run.sh` lists the covered paths that changed after the sync commit. If your change touches none of them and no new caller writes `provisioner_jobs`, the check does not apply.

## Run

```sh
.agents/skills/verify-provisioner-jobs/run.sh
```

It needs Java 11 or newer: `java` on `PATH`, `JAVA=<path>`, or `mise` (it then runs `mise exec java@temurin-25`). The first run downloads `tla2tools.jar` 1.7.4 into `build/verify-provisioner-jobs/` and checks its SHA-256. A run takes about 20 seconds. Set `OUT=<dir>` to keep the TLC logs in a folder you choose.

The script prints one line per config and ends with `checks: N ok, M failed, 9 total`. It exits 0 only when every config gives its expected result. A run without the count line did not finish.

A pass checks the spec, not your diff. When covered code changed after the sync commit, `run.sh` lists those paths and prints a `WARNING` line. Until you update the spec to match the change (see "When your change is intended"), a pass tells you nothing about it.

Read every log with the `/statechart` skill from the pstack plugin. It turns each violation into the shortest path of steps in plain words and draws the state graph of a passing config. Without it, read the trace in `$OUT/<config>.out`: TLC prints each state with the action that reached it.

## What it checks

| Property | The promise | Harm if broken | Code that keeps it | Config that breaks it |
|---|---|---|---|---|
| `OneRunnerPerJob` | At most one daemon runs a job. | Two Terraform runs on one workspace duplicate or corrupt infrastructure. | `AcquireProvisionerJob` picks the row `FOR UPDATE SKIP LOCKED` inside the `UPDATE` (`coderd/database/queries/provisionerjobs.sql`). | `NegAcquireNoLock` |
| `CancelNeverClosesRunningJob` | A cancel never sets `completed_at` while a daemon holds the job. | The next build starts while Terraform still runs. | `patchCancelWorkspaceBuild` sets `completed_at` only without a worker (`coderd/workspacebuilds.go`). The prebuild cancel runs at Repeatable Read (`enterprise/coderd/prebuilds/reconcile.go`). | `NegCancelIgnoresWorker`, `NegPrebuildCancelReadCommitted` |
| `ReaperSparesLiveJob` | The reaper fails only jobs whose `updated_at` is stale. | A healthy build is failed and the next one runs over it. | `reapJob` rechecks `updated_at` under the row lock (`coderd/jobreaper/detector.go`). | `NegReaperNoStaleRecheck` |
| `OutcomeIsFinal` | Once `completed_at` is set, the job's error and its build's state never change. | A failed build turns into a succeeded one, with state that a newer build never saw. | `reapJob` rechecks `completed_at` under the row lock. Nothing guards `CompleteJob` and `FailJob` today: finding F1. | `F1Outcome` (the code today), `NegReaperNoCompletedRecheck` |
| `NextBuildStartsFromFinalState` | Build N+1 starts from build N's final state. | Build N+1 plans against old state, so it duplicates or orphans cloud resources. | Nothing today: finding F1. | `F1NextBuild` (the code today) |

Configs:

- `Safety` is the code today with the three properties it keeps. It must pass.
- `F1Outcome` and `F1NextBuild` are the code today. They must break.
- `Fixed` turns on `CompleteGuardsCompleted`, the proposed F1 fix, and checks every property. It must pass.
- Each `Neg*` config turns off one guard that the code has. It must break its property. If it passes, the property no longer depends on that guard, and the model is wrong.

## Finding F1: a late daemon rewrites a reaped job

Status at the sync commit: open. `F1Outcome` reports this path as TLC states 2 to 8. `F1NextBuild` reports the same path with the next build started before the last step.

1. Daemon d1 acquires job 1 (`AcquireProvisionerJob`).
2. d1 freezes, for example because its process or VM is paused.
3. Five minutes pass with no heartbeat.
4. d1 resumes.
5. The reaper's `GetProvisionerJobsToBeReaped` returns job 1.
6. `reapJob` marks job 1 failed and copies build N-1's state into build N.
7. d1's `CompleteJob` checks only `worker_id` (`coderd/provisionerdserver/provisionerdserver.go` `CompleteJob`). Then `completeWorkspaceBuildJob` sets `error = NULL` and writes the new state.

Job 1 now reads `succeeded`. A build N+1 started after step 6 runs from build N-1's state. A heartbeat after the reap also succeeds and returns `canceled=false`, because `UpdateJob` does not read `completed_at`. `FailJob` checks `completed_at` with an unlocked read, so it can overwrite a reap in the same way. `completeTemplateImportJob` and `completeTemplateDryRunJob` make the same unguarded write. The model does not cover them.

`replay_f1_test.go.txt` replays these steps against Postgres. To run it, copy it to `coderd/provisionerdserver/zz_formal_replay_test.go` and run `go test ./coderd/provisionerdserver/ -run TestFormalReplayF1 -count=1 -v`. Then delete the copy. It passes while the bug exists. It logs `status=failed` after the reap, then `status=succeeded` after the late `CompleteJob`.

The model shows that this change makes both F1 properties hold: write `completed_at`, `error` and the state only `WHERE completed_at IS NULL`, and fail the RPC when no row changed.

When F1 is fixed:

1. Set `CompleteGuardsCompleted = TRUE` in `Safety.cfg`, and add `NextBuildStartsFromFinalState` and `PROPERTIES OutcomeIsFinal` to it.
2. Delete `F1Outcome.cfg`, `F1NextBuild.cfg` and the replay file, and remove the F1 configs from the `checks` list in `run.sh`.
3. Turn the replay into a regression test that expects the late `CompleteJob` to fail. That test belongs in the fix PR.

## A real bug or a modeling error

When a config gives a result its row in `checks` does not expect, before or after you edit the spec:

1. Read the trace with `/statechart`. Map each step to the code in the action's comment.
2. Replay the path against the real code as a Go test, as `replay_f1_test.go.txt` does. For daemon RPCs, use `setup` from `coderd/provisionerdserver/provisionerdserver_test.go`. For the reaper, use `newDetectorTestEnv` from `coderd/jobreaper/detector_test.go`, or `jobreaper.New` with a tick channel. Set `updated_at` in the past to stand in for time passing. To put a step between the reaper's scan and its locked transaction, wrap the `database.Store` and run the step when `GetProvisionerJobByIDForUpdate` is called.
3. If the real code does what the trace says, it is a finding. Report the path in the code's words. Do not weaken the property or the model to make it pass.
4. If the real code does not do it, the model is wrong. Find the action whose guard or atomicity differs from the code and fix it.

## When your change is intended

1. Find the actions that your change touches, using the code names in the spec comments. A new guard either matches an existing switch, or it needs a new action or variable.
2. Edit the spec so that it says what the new code does. Keep each action as large as one statement or one locked transaction in the code.
3. Run `run.sh`. The `Safety` config must still pass, and each `Neg*` config must still break its property. If one does not, follow "A real bug or a modeling error".
4. When you add a guard, add a switch for it and a `Neg*` config that turns it off. Add the config's row to `checks` in `run.sh`. When you remove part of a guard, give each part its own switch first, as `ReaperRechecksCompleted` and `ReaperRechecksStale` do.
5. When `run.sh` reports covered paths that changed after the sync commit, read their diffs against the model. Then set `sync` in `coverage.txt` to the commit you checked. Move it only after you confirm the model describes that commit.

## Assumptions and limits

- Bounds: two daemons, two builds of one workspace, and every interleaving of their steps. `Safety` explores about 29,000 distinct states.
- Time is one flag per job: `updated_at` is older than the reaper threshold. A job goes stale only when no live process holds it. That means the job is pending, its daemon crashed, or its daemon is frozen. A live runner heartbeats every few seconds, and after one failed 30 second update it stops and sends `FailJob` (`provisionerd/runner`). A frozen daemon stands for a paused process or VM, or for replica clocks skewed past five minutes. The model does not check whether real clock skew reaches that.
- Postgres: the default isolation is Read Committed. A row lock serializes the writers of a row. `SKIP LOCKED` skips a row that another transaction holds. Repeatable Read fails a statement whose row changed after its snapshot.
- The model leaves out these parts:
  - liveness (whether every job finishes)
  - pubsub delivery
  - logs, quota, resources and agents
  - provisioner tags and organizations
  - the acquirer's in-memory queue
  - template import and dry-run jobs
  - more than two builds or daemons
