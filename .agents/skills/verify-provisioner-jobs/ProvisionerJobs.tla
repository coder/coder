---------------------------- MODULE ProvisionerJobs ----------------------------
(***************************************************************************)
(* Provisioner job lifecycle for two consecutive builds of one workspace,   *)
(* build N (job 1) and build N+1 (job 2). Actors: provisioner daemons, the  *)
(* job reaper, user cancel, and the prebuilds reconciler's pending cancel.  *)
(* Every action is one Postgres statement or one transaction that holds    *)
(* the job row lock, so it is indivisible in the code too. Steps that the  *)
(* code splits across statements without a lock are separate actions.     *)
(*                                                                         *)
(* Paths below are relative to the repository root.                        *)
(* pj.sql  = coderd/database/queries/provisionerjobs.sql                   *)
(* pds.go  = coderd/provisionerdserver/provisionerdserver.go               *)
(* det.go  = coderd/jobreaper/detector.go                                  *)
(* wsb.go  = coderd/workspacebuilds.go                                     *)
(* rec.go  = enterprise/coderd/prebuilds/reconcile.go                      *)
(***************************************************************************)
EXTENDS Naturals

CONSTANTS
    Daemons,
    None,
    \* TRUE in the code today. pj.sql AcquireProvisionerJob selects the job
    \* with FOR UPDATE SKIP LOCKED inside the UPDATE, so pick and claim are
    \* one step. FALSE models a plain subquery: two daemons can pick the
    \* same row and both claim it.
    AcquireLocksRow,
    \* TRUE in the code today. wsb.go patchCancelWorkspaceBuild sets
    \* completed_at only when worker_id is NULL.
    CancelChecksWorker,
    \* TRUE in the code today. rec.go cancelAndOrphanDeletePendingPrebuilds
    \* runs UpdatePrebuildProvisionerJobWithCancel at LevelRepeatableRead, so
    \* a row that changed after the statement snapshot fails the statement.
    PrebuildCancelRepeatableRead,
    \* TRUE in the code today. det.go reapJob rechecks completed_at after
    \* GetProvisionerJobByIDForUpdate.
    ReaperRechecksCompleted,
    \* TRUE in the code today. det.go reapJob then rechecks that updated_at
    \* is still older than the threshold.
    ReaperRechecksStale,
    \* How CompleteJob and FailJob treat a job that is already completed,
    \* for example by the reaper.
    \* "wins": the code today. CompleteJob writes the outcome and the state.
    \* FailJob writes too when the job completes after its unlocked read.
    \* "state": the outcome stays. The daemon still records its Terraform
    \* state while the build is the workspace's latest build.
    \* "rejected": both write only while completed_at IS NULL.
    LateWrite,
    \* TRUE in the code today. FailJob refuses a completed job, but first
    \* saves the state that a failed apply sends while the build is the
    \* workspace's latest build (pds.go saveStateOfCompletedJob).
    LateFailRecordsState

Jobs == {1, 2}

\* Daemon phases. "acquiring" exists only when AcquireLocksRow is FALSE.
\* "failing" is the FailJob read (pds.go FailJob GetProvisionerJobByID),
\* "failwrite" its later UpdateProvisionerJobWithCompleteByID.
Phases == {"idle", "acquiring", "running", "frozen", "failing", "failwrite"}
Holding == {"running", "frozen", "failing", "failwrite"}

\* provisioner_jobs.error: none, set by the daemon (FailJob), or by the reaper.
Errors == {"none", "daemon", "reaper"}

\* workspace_builds.provisioner_state: empty, copied from the build before
\* (det.go reapJob), or written by a daemon after Terraform ran.
States == {"empty", "copied", "real"}

VARIABLES
    exists,      \* job row inserted (job 2 only after job 1 is inactive)
    started,     \* started_at IS NOT NULL
    worker,      \* worker_id
    completed,   \* completed_at IS NOT NULL
    canceled,    \* canceled_at IS NOT NULL
    err,         \* error
    stale,       \* updated_at older than the reaper threshold
    state,       \* workspace_builds.provisioner_state of the job's build
    nextBase,    \* state build N+1 was created from (wsbuilder getState)
    phase,       \* daemon phase
    djob,        \* job the daemon works on
    reapQueue,   \* jobs the reaper's unlocked scan returned
    pcSnap,      \* prebuild cancel statement snapshot per job, or None
    reapedLive,  \* history: reaper failed the job while updated_at was fresh
    applied,     \* history: the daemon's Terraform apply finished
    cancelClosedRunning \* history: a cancel set completed_at while a worker held it

vars == <<exists, started, worker, completed, canceled, err, stale, state,
          nextBase, phase, djob, reapQueue, pcSnap, reapedLive,
          cancelClosedRunning, applied>>

jobVars == <<exists, started, worker, completed, canceled, err, stale, state>>

Row(j) == [started |-> started[j], worker |-> worker[j],
           completed |-> completed[j], canceled |-> canceled[j]]

\* A live daemon process heartbeats every few seconds (provisionerd/runner
\* heartbeatRoutine) and gives up after one failed 30 s update. So
\* updated_at can only go stale when no live process holds the job: the
\* holder crashed, or it is frozen (process or VM paused, or replica clocks
\* skewed past the 5 minute threshold, det.go HungJobDuration).
LiveHolder(j) == \E d \in Daemons : djob[d] = j /\ phase[d] \in {"running", "failing", "failwrite"}

TypeOK ==
    /\ exists \in [Jobs -> BOOLEAN]
    /\ started \in [Jobs -> BOOLEAN]
    /\ worker \in [Jobs -> Daemons \cup {None}]
    /\ completed \in [Jobs -> BOOLEAN]
    /\ canceled \in [Jobs -> BOOLEAN]
    /\ err \in [Jobs -> Errors]
    /\ stale \in [Jobs -> BOOLEAN]
    /\ state \in [Jobs -> States]
    /\ nextBase \in States \cup {None}
    /\ phase \in [Daemons -> Phases]
    /\ djob \in [Daemons -> Jobs \cup {None}]
    /\ reapQueue \subseteq Jobs
    /\ reapedLive \in [Jobs -> BOOLEAN]
    /\ cancelClosedRunning \in [Jobs -> BOOLEAN]
    /\ applied \in [Jobs -> BOOLEAN]
    /\ LateWrite \in {"wins", "state", "rejected"}
    /\ LateFailRecordsState \in BOOLEAN

Init ==
    /\ exists = [j \in Jobs |-> j = 1]
    /\ started = [j \in Jobs |-> FALSE]
    /\ worker = [j \in Jobs |-> None]
    /\ completed = [j \in Jobs |-> FALSE]
    /\ canceled = [j \in Jobs |-> FALSE]
    /\ err = [j \in Jobs |-> "none"]
    /\ stale = [j \in Jobs |-> FALSE]
    /\ state = [j \in Jobs |-> "empty"]
    /\ nextBase = None
    /\ phase = [d \in Daemons |-> "idle"]
    /\ djob = [d \in Daemons |-> None]
    /\ reapQueue = {}
    /\ pcSnap = [j \in Jobs |-> None]
    /\ reapedLive = [j \in Jobs |-> FALSE]
    /\ cancelClosedRunning = [j \in Jobs |-> FALSE]
    /\ applied = [j \in Jobs |-> FALSE]

Acquirable(j) == exists[j] /\ ~started[j] /\ ~completed[j]

\* The build is the workspace's latest build.
Latest(j) == j = 2 \/ ~exists[2]

Claim(d, j) ==
    /\ started' = [started EXCEPT ![j] = TRUE]
    /\ worker' = [worker EXCEPT ![j] = d]
    /\ stale' = [stale EXCEPT ![j] = FALSE]
    /\ phase' = [phase EXCEPT ![d] = "running"]
    /\ djob' = [djob EXCEPT ![d] = j]

\* pj.sql AcquireProvisionerJob, one UPDATE whose subquery locks the row.
DaemonAcquires(d) ==
    /\ AcquireLocksRow
    /\ phase[d] = "idle"
    /\ \E j \in Jobs :
        /\ Acquirable(j)
        /\ Claim(d, j)
    /\ UNCHANGED <<exists, completed, canceled, err, state, nextBase,
                   reapQueue, pcSnap, reapedLive, cancelClosedRunning, applied>>

\* Broken variant: the subquery reads without a lock ...
DaemonPicksJob(d) ==
    /\ ~AcquireLocksRow
    /\ phase[d] = "idle"
    /\ \E j \in Jobs :
        /\ Acquirable(j)
        /\ phase' = [phase EXCEPT ![d] = "acquiring"]
        /\ djob' = [djob EXCEPT ![d] = j]
    /\ UNCHANGED <<jobVars, nextBase, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* ... and the outer UPDATE matches only on id, so it claims the row anyway.
DaemonClaimsPickedJob(d) ==
    /\ phase[d] = "acquiring"
    /\ Claim(d, djob[d])
    /\ UNCHANGED <<exists, completed, canceled, err, state, nextBase,
                   reapQueue, pcSnap, reapedLive, cancelClosedRunning, applied>>

\* pds.go UpdateJob: ownership check, then UpdateProvisionerJobByID sets
\* updated_at. It never looks at completed_at.
DaemonHeartbeats(d) ==
    /\ phase[d] = "running"
    /\ worker[djob[d]] = d
    /\ stale' = [stale EXCEPT ![djob[d]] = FALSE]
    /\ UNCHANGED <<exists, started, worker, completed, canceled, err, state,
                   nextBase, phase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

DaemonFreezes(d) ==
    /\ phase[d] = "running"
    /\ phase' = [phase EXCEPT ![d] = "frozen"]
    /\ UNCHANGED <<jobVars, nextBase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

DaemonThaws(d) ==
    /\ phase[d] = "frozen"
    /\ phase' = [phase EXCEPT ![d] = "running"]
    /\ UNCHANGED <<jobVars, nextBase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* The daemon process dies. Its job stays started with no live holder. A
\* crash after an apply loses the state with the process, so the model
\* lets a daemon crash only before its apply ran.
DaemonCrashes(d) ==
    /\ phase[d] /= "idle"
    /\ ~applied[djob[d]]
    /\ phase' = [phase EXCEPT ![d] = "idle"]
    /\ djob' = [djob EXCEPT ![d] = None]
    /\ UNCHANGED <<jobVars, nextBase, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* Terraform finished. pds.go CompleteJob checks only worker_id, then
\* completeWorkspaceBuildJob runs, in one transaction,
\* UpdateProvisionerJobWithCompleteByID (completed_at and updated_at = now,
\* error = NULL)
\* and UpdateWorkspaceBuildProvisionerStateByID. worker_id never changes
\* after acquire, so the unlocked read before the transaction cannot go
\* stale in a way that matters. When CompleteJob returns an error, the
\* runner sends FailJob (provisionerd/runner Run).
DaemonCompletesJob(d) ==
    LET j == djob[d] IN
    /\ phase[d] = "running"
    /\ worker[j] = d
    /\ applied' = [applied EXCEPT ![j] = TRUE]
    /\ CASE ~completed[j] \/ LateWrite = "wins" ->
              /\ completed' = [completed EXCEPT ![j] = TRUE]
              /\ err' = [err EXCEPT ![j] = "none"]
              /\ state' = [state EXCEPT ![j] = "real"]
              /\ stale' = [stale EXCEPT ![j] = FALSE]
              /\ phase' = [phase EXCEPT ![d] = "idle"]
              /\ djob' = [djob EXCEPT ![d] = None]
              /\ UNCHANGED <<exists, started, worker, canceled>>
         [] LateWrite = "state" ->
              /\ state' = [state EXCEPT ![j] = IF Latest(j) THEN "real" ELSE @]
              /\ phase' = [phase EXCEPT ![d] = "idle"]
              /\ djob' = [djob EXCEPT ![d] = None]
              /\ UNCHANGED <<exists, started, worker, completed, canceled, err, stale>>
         [] LateWrite = "rejected" ->
              \* CompleteJob fails. The runner's fallback FailJob carries no
              \* state (runner.go Run, failedJobf), and FailJob refuses the
              \* completed job, so nothing is written.
              /\ phase' = [phase EXCEPT ![d] = "idle"]
              /\ djob' = [djob EXCEPT ![d] = None]
              /\ UNCHANGED jobVars
    /\ UNCHANGED <<nextBase, reapQueue, pcSnap, reapedLive, cancelClosedRunning>>

\* Terraform failed, a heartbeat failed, or a cancel timed out: the runner
\* sends FailJob. When the apply failed after it made resources, applied
\* becomes TRUE and FailJob carries the state Terraform wrote
\* (provisionerd/runner/runner.go runWorkspaceBuild). The FailJob after a
\* failed heartbeat or a cancel timeout carries no state (failedJobf). The
\* model does not give those an apply.
DaemonStartsFailJob(d) ==
    LET j == djob[d] IN
    /\ phase[d] = "running"
    /\ phase' = [phase EXCEPT ![d] = "failing"]
    /\ \E a \in BOOLEAN : applied' = [applied EXCEPT ![j] = @ \/ a]
    /\ UNCHANGED <<jobVars, nextBase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning>>

\* The state a FailJob request carries. After an apply ran, it carries the
\* state Terraform wrote. Otherwise it carries none, or a state equal to
\* the one the build has.
FailState(j) == IF applied[j] THEN {"real"} ELSE {state[j]}

\* pds.go FailJob reads the job without a lock and refuses a completed job.
\* With LateFailRecordsState it saves the state of the latest build first.
FailJobReadsJob(d) ==
    LET j == djob[d] IN
    /\ phase[d] = "failing"
    /\ IF completed[j]
       THEN /\ phase' = [phase EXCEPT ![d] = "idle"]
            /\ djob' = [djob EXCEPT ![d] = None]
            /\ IF LateFailRecordsState /\ Latest(j)
               THEN \E s \in FailState(j) : state' = [state EXCEPT ![j] = s]
               ELSE UNCHANGED state
            /\ UNCHANGED <<exists, started, worker, completed, canceled, err, stale>>
       ELSE /\ phase' = [phase EXCEPT ![d] = "failwrite"]
            /\ UNCHANGED <<jobVars, djob>>
    /\ UNCHANGED <<nextBase, reapQueue, pcSnap, reapedLive, cancelClosedRunning, applied>>

\* pds.go FailJob then writes UpdateProvisionerJobWithCompleteByID by id
\* only, and UpdateWorkspaceBuildProvisionerStateByID when the daemon sent
\* state.
FailJobWrites(d) ==
    LET j == djob[d] IN
    /\ phase[d] = "failwrite"
    /\ phase' = [phase EXCEPT ![d] = "idle"]
    /\ djob' = [djob EXCEPT ![d] = None]
    /\ CASE ~completed[j] \/ LateWrite = "wins" ->
              /\ completed' = [completed EXCEPT ![j] = TRUE]
              /\ err' = [err EXCEPT ![j] = "daemon"]
              /\ \E s \in FailState(j) : state' = [state EXCEPT ![j] = s]
              /\ stale' = [stale EXCEPT ![j] = FALSE]
              /\ UNCHANGED <<exists, started, worker, canceled>>
         [] LateWrite = "state" ->
              /\ IF Latest(j)
                 THEN \E s \in FailState(j) : state' = [state EXCEPT ![j] = s]
                 ELSE UNCHANGED state
              /\ UNCHANGED <<exists, started, worker, completed, canceled, err, stale>>
         [] LateWrite = "rejected" -> UNCHANGED jobVars
    /\ UNCHANGED <<nextBase, reapQueue, pcSnap, reapedLive, cancelClosedRunning, applied>>

\* Time passes past the reaper threshold for a job no live process holds:
\* pending for 30 minutes, or running with a crashed or frozen holder.
TimePasses(j) ==
    /\ exists[j]
    /\ ~completed[j]
    /\ ~stale[j]
    /\ ~LiveHolder(j)
    /\ stale' = [stale EXCEPT ![j] = TRUE]
    /\ UNCHANGED <<exists, started, worker, completed, canceled, err, state,
                   nextBase, phase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* det.go run: GetProvisionerJobsToBeReaped, a plain SELECT.
ReaperScans(j) ==
    /\ exists[j]
    /\ ~completed[j]
    /\ stale[j]
    /\ j \notin reapQueue
    /\ reapQueue' = reapQueue \cup {j}
    /\ UNCHANGED <<jobVars, nextBase, phase, djob, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* det.go reapJob: one transaction that holds the row lock
\* (GetProvisionerJobByIDForUpdate), rechecks the job, marks it failed with
\* UpdateProvisionerJobWithCompleteWithStartedAtByID, and copies the build
\* before's state into an empty build state.
ReaperReapsJob(j) ==
    /\ j \in reapQueue
    /\ reapQueue' = reapQueue \ {j}
    /\ IF (~ReaperRechecksCompleted \/ ~completed[j]) /\ (~ReaperRechecksStale \/ stale[j])
       THEN /\ completed' = [completed EXCEPT ![j] = TRUE]
            /\ started' = [started EXCEPT ![j] = TRUE]
            /\ err' = [err EXCEPT ![j] = "reaper"]
            /\ state' = [state EXCEPT ![j] = IF @ = "empty" THEN "copied" ELSE @]
            /\ stale' = [stale EXCEPT ![j] = FALSE]
            /\ reapedLive' = [reapedLive EXCEPT ![j] = @ \/ ~stale[j]]
            /\ UNCHANGED <<exists, worker, canceled>>
       ELSE UNCHANGED <<jobVars, reapedLive>>
    /\ UNCHANGED <<nextBase, phase, djob, pcSnap, cancelClosedRunning, applied>>

\* wsb.go patchCancelWorkspaceBuild: one transaction with
\* GetProvisionerJobByIDWithLock (FOR UPDATE), refusing completed or
\* canceled jobs, then UpdateProvisionerJobWithCancelByID.
UserCancels(j) ==
    /\ exists[j]
    /\ ~completed[j]
    /\ ~canceled[j]
    /\ canceled' = [canceled EXCEPT ![j] = TRUE]
    /\ IF ~CancelChecksWorker \/ worker[j] = None
       THEN /\ completed' = [completed EXCEPT ![j] = TRUE]
            /\ cancelClosedRunning' = [cancelClosedRunning EXCEPT ![j] = @ \/ worker[j] /= None]
       ELSE UNCHANGED <<completed, cancelClosedRunning, applied>>
    /\ UNCHANGED <<exists, started, worker, err, stale, state, nextBase,
                   phase, djob, reapQueue, pcSnap, reapedLive, applied>>

\* prebuilds.sql UpdatePrebuildProvisionerJobWithCancel: the CTE picks
\* pending jobs with worker_id IS NULL from the statement snapshot ...
PrebuildCancelSelects(j) ==
    /\ exists[j]
    /\ ~started[j]
    /\ ~completed[j]
    /\ ~canceled[j]
    /\ worker[j] = None
    /\ pcSnap[j] = None
    /\ pcSnap' = [pcSnap EXCEPT ![j] = Row(j)]
    /\ UNCHANGED <<jobVars, nextBase, phase, djob, reapQueue, reapedLive,
                   cancelClosedRunning, applied>>

\* ... and the UPDATE joins on id only. Under Repeatable Read a row that
\* changed after the snapshot fails the statement. Under Read Committed the
\* CTE result is not rechecked, so the write goes through.
PrebuildCancelWrites(j) ==
    /\ pcSnap[j] /= None
    /\ pcSnap' = [pcSnap EXCEPT ![j] = None]
    /\ IF PrebuildCancelRepeatableRead /\ Row(j) /= pcSnap[j]
       THEN UNCHANGED <<jobVars, cancelClosedRunning, applied>>
       ELSE /\ canceled' = [canceled EXCEPT ![j] = TRUE]
            /\ completed' = [completed EXCEPT ![j] = TRUE]
            /\ cancelClosedRunning' = [cancelClosedRunning EXCEPT ![j] = @ \/ worker[j] /= None]
            /\ UNCHANGED <<exists, started, worker, err, stale, state>>
    /\ UNCHANGED <<nextBase, phase, djob, reapQueue, reapedLive, applied>>

\* coderd/wsbuilder checkRunningBuild refuses a new build while the latest
\* job is pending, running or canceling, which all have completed_at NULL.
\* getState copies the latest build's provisioner_state into the new build.
UserStartsNextBuild ==
    /\ ~exists[2]
    /\ completed[1]
    /\ exists' = [exists EXCEPT ![2] = TRUE]
    /\ nextBase' = state[1]
    /\ UNCHANGED <<started, worker, completed, canceled, err, stale, state,
                   phase, djob, reapQueue, pcSnap, reapedLive,
                   cancelClosedRunning, applied>>

\* Both builds are finished and every daemon is idle.
AllDone ==
    /\ \A j \in Jobs : exists[j] /\ completed[j]
    /\ \A d \in Daemons : phase[d] = "idle"
    /\ reapQueue = {}
    /\ \A j \in Jobs : pcSnap[j] = None
    /\ UNCHANGED vars

Next ==
    \/ \E d \in Daemons :
        \/ DaemonAcquires(d)
        \/ DaemonPicksJob(d)
        \/ DaemonClaimsPickedJob(d)
        \/ DaemonHeartbeats(d)
        \/ DaemonFreezes(d)
        \/ DaemonThaws(d)
        \/ DaemonCrashes(d)
        \/ DaemonCompletesJob(d)
        \/ DaemonStartsFailJob(d)
        \/ FailJobReadsJob(d)
        \/ FailJobWrites(d)
    \/ \E j \in Jobs :
        \/ TimePasses(j)
        \/ ReaperScans(j)
        \/ ReaperReapsJob(j)
        \/ UserCancels(j)
        \/ PrebuildCancelSelects(j)
        \/ PrebuildCancelWrites(j)
    \/ UserStartsNextBuild
    \/ AllDone

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* Properties. The first four are promises the code makes. The last two  *)
(* are not, and run.sh expects TLC to report them broken.                  *)

\* At most one daemon runs a job. Guarded by AcquireLocksRow.
OneRunnerPerJob ==
    \A d1, d2 \in Daemons :
        (d1 /= d2 /\ phase[d1] \in Holding /\ phase[d2] \in Holding)
            => djob[d1] /= djob[d2]

\* A cancel never marks a job complete while a daemon holds it, so the
\* daemon can still report what Terraform did. Guarded by CancelChecksWorker
\* and PrebuildCancelRepeatableRead.
CancelNeverClosesRunningJob == \A j \in Jobs : ~cancelClosedRunning[j]

\* The reaper fails only jobs whose updated_at is stale. Guarded by
\* ReaperRechecksStale.
ReaperSparesLiveJob == \A j \in Jobs : ~reapedLive[j]

\* Once completed_at is set, the job's outcome never changes. The code does
\* not promise this: with LateWrite = "wins", a late CompleteJob marks a
\* reaped job succeeded. ReaperRechecksCompleted keeps the reaper itself
\* from changing an outcome.
OutcomeIsFinal ==
    [][\A j \in Jobs :
        completed[j] =>
            /\ completed'[j]
            /\ err'[j] = err[j]
            /\ canceled'[j] = canceled[j]]_vars

\* When a daemon's apply finished and nothing still holds the job, the
\* latest build records the state that Terraform wrote, so the next build
\* plans against the resources that exist.
LatestStateTracksApply ==
    \A j \in Jobs :
        (applied[j] /\ Latest(j) /\ \A d \in Daemons : djob[d] /= j)
            => state[j] = "real"

\* Build N+1 starts from a state that includes build N's finished apply.
\* The code does not promise this, and no LateWrite mode keeps it: the
\* reaper unblocks build N+1 while a frozen daemon still holds build N, so
\* the apply can finish after build N+1 copied its state.
NextBuildSeesApply == (exists[2] /\ applied[1]) => nextBase = "real"

=============================================================================
