package database

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AcquireWorkspaceAgentSubagentExecutionParams keeps the public Store
// parameters stable while the individual statements behind the acquisition
// remain internal generated queries.
type AcquireWorkspaceAgentSubagentExecutionParams struct {
	WorkspaceBuildID uuid.UUID `db:"workspace_build_id" json:"workspace_build_id"`
	DeclarationID    uuid.UUID `db:"declaration_id" json:"declaration_id"`
	ParentAgentID    uuid.UUID `db:"parent_agent_id" json:"parent_agent_id"`
	Now              time.Time `db:"now" json:"now"`
}

// AcquireWorkspaceAgentSubagentExecutionRow is the row returned by the
// acquisition. It is the row of the statement that performs the mutation, so the
// public surface cannot drift from what the database actually returns.
type AcquireWorkspaceAgentSubagentExecutionRow = MarkWorkspaceAgentSubagentExecutionAcquiredRow

// AcquireWorkspaceAgentSubagentExecution hands a parent agent the credentials it
// needs to launch one declared subagent execution, and fences every previous
// launcher of the same declaration.
//
// The caller supplies the execution tuple it believes it owns. The acquisition
// only succeeds when all of the following hold:
//
//   - the exact (workspace_build_id, declaration_id, parent_agent_id) tuple
//     exists, so a parent cannot acquire another parent's declaration;
//   - a status row exists and is not 'stopping', so a shutting-down execution is
//     never restarted;
//   - the workspace's actual latest build, resolved here instead of trusted from
//     the caller or a cached middleware build, is the requested build and has
//     transition 'start', so a stale generation cannot launch;
//   - the parent is live, top-level, and part of the requested build;
//   - the child is exactly the persisted child_agent_id, live, a direct child of
//     the exact parent, on the parent's resource, and execution isolated.
//
// The statements run in one transaction at READ COMMITTED, where every statement
// takes its own snapshot. A single multi-CTE statement cannot express this
// safely: all of its CTEs share one snapshot, so the validation could read a
// generation that a concurrently committing build had already replaced. The
// sequence therefore is:
//
//  1. read the execution's immutable identity (owning workspace, declared child);
//  2. take the workspace build publication advisory lock, which every
//     workspace_builds insert also takes through the
//     serialize_workspace_build_publication trigger, so no new generation for
//     this workspace can become visible for the rest of the transaction;
//  3. lock the status row FOR UPDATE, which serializes concurrent acquisitions
//     of the same declaration and makes acquisition_version strictly increasing;
//  4. resolve the workspace's latest build in a fresh statement and require it
//     to be the requested build with transition 'start';
//  5. re-read the parent under the lock and require it to still be live,
//     top-level, and part of the requested build;
//  6. lock the child FOR SHARE, which conflicts with an ordinary soft-delete
//     update and forces an EvalPlanQual recheck, so a child removed by a
//     committing rebuild cannot be handed out;
//  7. record the acquisition on the already-locked status row.
//
// Lock order is always publication guard, then execution status, then the exact
// child row. Callers must invoke this standalone rather than from an outer
// long-lived transaction, otherwise the publication guard is held for the
// lifetime of that outer transaction.
//
// Any validation failure returns sql.ErrNoRows, so callers cannot distinguish a
// stale generation from a declaration they do not own.
func (q *sqlQuerier) AcquireWorkspaceAgentSubagentExecution(ctx context.Context, arg AcquireWorkspaceAgentSubagentExecutionParams) (AcquireWorkspaceAgentSubagentExecutionRow, error) {
	var acquired AcquireWorkspaceAgentSubagentExecutionRow
	err := q.InTx(func(tx Store) error {
		execution, err := tx.GetWorkspaceAgentSubagentExecutionForAcquisition(ctx, GetWorkspaceAgentSubagentExecutionForAcquisitionParams{
			WorkspaceBuildID: arg.WorkspaceBuildID,
			DeclarationID:    arg.DeclarationID,
			ParentAgentID:    arg.ParentAgentID,
		})
		if err != nil {
			return xerrors.Errorf("get subagent execution for acquisition: %w", err)
		}

		if err := tx.AcquireWorkspaceBuildPublicationLock(ctx, execution.WorkspaceID); err != nil {
			return xerrors.Errorf("acquire workspace build publication lock: %w", err)
		}

		if _, err := tx.LockWorkspaceAgentSubagentExecutionStatusForAcquisition(ctx, LockWorkspaceAgentSubagentExecutionStatusForAcquisitionParams{
			WorkspaceBuildID: arg.WorkspaceBuildID,
			DeclarationID:    arg.DeclarationID,
		}); err != nil {
			return xerrors.Errorf("lock subagent execution status: %w", err)
		}

		latest, err := tx.GetLatestWorkspaceBuildGeneration(ctx, execution.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get latest workspace build generation: %w", err)
		}
		// The requested build must still be the workspace's current generation.
		// A rejection is indistinguishable from a missing row on purpose.
		if latest.ID != arg.WorkspaceBuildID || latest.Transition != WorkspaceTransitionStart {
			return sql.ErrNoRows
		}

		parent, err := tx.GetWorkspaceAgentSubagentExecutionAcquisitionParent(ctx, GetWorkspaceAgentSubagentExecutionAcquisitionParentParams{
			ParentAgentID:    arg.ParentAgentID,
			WorkspaceBuildID: arg.WorkspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("get subagent execution acquisition parent: %w", err)
		}

		child, err := tx.LockWorkspaceAgentSubagentExecutionChildForAcquisition(ctx, LockWorkspaceAgentSubagentExecutionChildForAcquisitionParams{
			ChildAgentID:  execution.ChildAgentID,
			ParentAgentID: uuid.NullUUID{UUID: parent.ID, Valid: true},
			ResourceID:    parent.ResourceID,
		})
		if err != nil {
			return xerrors.Errorf("lock subagent execution child: %w", err)
		}

		acquired, err = tx.MarkWorkspaceAgentSubagentExecutionAcquired(ctx, MarkWorkspaceAgentSubagentExecutionAcquiredParams{
			Now:              arg.Now,
			WorkspaceBuildID: arg.WorkspaceBuildID,
			DeclarationID:    arg.DeclarationID,
			ChildAgentID:     child.ID,
		})
		if err != nil {
			return xerrors.Errorf("mark subagent execution acquired: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return AcquireWorkspaceAgentSubagentExecutionRow{}, err
	}
	return acquired, nil
}

// SubagentExecutionStatus is the lifecycle state stored in
// workspace_agent_subagent_execution_statuses.status. The column is text with a
// CHECK constraint rather than a Postgres enum, so these constants are the only
// values the database accepts.
type SubagentExecutionStatus string

const (
	// SubagentExecutionStatusPending is the state a declaration is inserted in.
	// No launcher owns it yet, so nothing may be reported on it.
	SubagentExecutionStatusPending SubagentExecutionStatus = "pending"
	// SubagentExecutionStatusStarting is owned by the acquisition: only
	// AcquireWorkspaceAgentSubagentExecution moves a declaration into it, so it
	// is never an acceptable reported status.
	SubagentExecutionStatusStarting SubagentExecutionStatus = "starting"
	// SubagentExecutionStatusRunning means the launcher considers the child up.
	SubagentExecutionStatusRunning SubagentExecutionStatus = "running"
	// SubagentExecutionStatusStopping means the launcher is shutting the child
	// down. The acquisition refuses to restart a declaration in this state.
	SubagentExecutionStatusStopping SubagentExecutionStatus = "stopping"
	// SubagentExecutionStatusStopped is terminal for a launcher: the child is
	// gone and was not lost to an error.
	SubagentExecutionStatusStopped SubagentExecutionStatus = "stopped"
	// SubagentExecutionStatusFailed is terminal for a launcher: the child is
	// gone because something went wrong. A new acquisition, not a report, is
	// what moves the declaration out of it.
	SubagentExecutionStatusFailed SubagentExecutionStatus = "failed"
)

// reportableSubagentExecutionStatuses is the report transition matrix, keyed by
// the status currently stored for the execution. A status is listed as its own
// successor where repeating it must be idempotent, which is what lets a launcher
// resend its state after a reconnect without rewriting status_changed_at.
//
// Two rules shape the matrix:
//
//   - 'starting' appears in no successor list, because the acquisition owns that
//     state. A launcher that wants to be 'starting' again must acquire again,
//     which is the only way to bump the fencing acquisition_version.
//   - 'pending' has no successors. A pending declaration has never been
//     acquired, so it has acquisition_version 0 and the fencing predicate
//     already rejects any report against it. The empty entry records that there
//     is no exception to that.
var reportableSubagentExecutionStatuses = map[SubagentExecutionStatus][]SubagentExecutionStatus{
	SubagentExecutionStatusPending: {},
	SubagentExecutionStatusStarting: {
		SubagentExecutionStatusRunning,
		SubagentExecutionStatusStopping,
		SubagentExecutionStatusFailed,
	},
	SubagentExecutionStatusRunning: {
		SubagentExecutionStatusRunning,
		SubagentExecutionStatusStopping,
		SubagentExecutionStatusStopped,
		SubagentExecutionStatusFailed,
	},
	SubagentExecutionStatusStopping: {
		SubagentExecutionStatusStopping,
		SubagentExecutionStatusStopped,
		SubagentExecutionStatusFailed,
	},
	SubagentExecutionStatusStopped: {SubagentExecutionStatusStopped},
	SubagentExecutionStatusFailed:  {SubagentExecutionStatusFailed},
}

// stoppedGenerationSubagentExecutionStatuses are the only statuses a launcher may
// report while the workspace's latest generation is a 'stop' build that replaced
// the reported one. The launcher is being torn down, so it may record that it is
// going away, but it may never claim the child is up on a superseded generation.
var stoppedGenerationSubagentExecutionStatuses = []SubagentExecutionStatus{
	SubagentExecutionStatusStopping,
	SubagentExecutionStatusStopped,
	SubagentExecutionStatusFailed,
}

// ReportWorkspaceAgentSubagentExecutionStatusParams keeps the public Store
// parameters stable while the individual statements behind the report remain
// internal generated queries.
type ReportWorkspaceAgentSubagentExecutionStatusParams struct {
	WorkspaceBuildID   uuid.UUID               `db:"workspace_build_id" json:"workspace_build_id"`
	DeclarationID      uuid.UUID               `db:"declaration_id" json:"declaration_id"`
	ParentAgentID      uuid.UUID               `db:"parent_agent_id" json:"parent_agent_id"`
	AcquisitionVersion int64                   `db:"acquisition_version" json:"acquisition_version"`
	Status             SubagentExecutionStatus `db:"status" json:"status"`
	LastError          string                  `db:"last_error" json:"last_error"`
	Now                time.Time               `db:"now" json:"now"`
}

// ReportWorkspaceAgentSubagentExecutionStatusRow is the row returned by the
// report. It is the row of the statement that performs the mutation, so the
// public surface cannot drift from what the database actually returns. It carries
// no credentials: a launcher reporting status has no reason to re-read the child
// auth token.
type ReportWorkspaceAgentSubagentExecutionStatusRow = WorkspaceAgentSubagentExecutionStatus

// ReportWorkspaceAgentSubagentExecutionStatus records the launcher-observed state
// of one declared subagent execution.
//
// The caller supplies the execution tuple it believes it owns plus the
// acquisition_version it was handed by AcquireWorkspaceAgentSubagentExecution.
// The report only succeeds when all of the following hold:
//
//   - the exact (workspace_build_id, declaration_id, parent_agent_id) tuple
//     exists, so a parent cannot report on another parent's declaration;
//   - the stored acquisition_version is exactly the requested one and is greater
//     than zero, so a fenced launcher cannot overwrite the state of the launcher
//     that replaced it, and a declaration nobody has acquired stays pending;
//   - the reported status is a permitted successor of the stored status, where
//     repeating the stored status is always permitted and 'starting' never is;
//   - the workspace's actual latest build, resolved here instead of trusted from
//     the caller, either is the requested build with transition 'start', or is a
//     'stop' build that replaced the requested build, in which case only
//     'stopping', 'stopped', and 'failed' may be reported;
//   - the parent is live, top-level, and part of the requested build;
//   - the child is exactly the persisted child_agent_id, live, a direct child of
//     the exact parent, on the parent's resource, and execution isolated.
//
// The statements run in one transaction at READ COMMITTED, where every statement
// takes its own snapshot, for the same reason the acquisition does: a single
// multi-CTE statement would validate the generation against one snapshot that a
// concurrently committing build could already have replaced. The lock order is
// the acquisition's order, publication guard, then execution status, then the
// exact child row, so the two paths can never deadlock against each other.
//
// The mutation writes only the reported facts. acquisition_version,
// restart_count, and last_acquired_at belong to the acquisition and are left
// untouched, and status_changed_at only moves when the status actually changes.
//
// Any validation failure returns sql.ErrNoRows, so callers cannot distinguish a
// stale generation, a fenced launcher, and a declaration they do not own. A
// last_error longer than the column's 4096 byte limit is a check violation
// instead, and the transaction is rolled back with no partial mutation.
func (q *sqlQuerier) ReportWorkspaceAgentSubagentExecutionStatus(ctx context.Context, arg ReportWorkspaceAgentSubagentExecutionStatusParams) (ReportWorkspaceAgentSubagentExecutionStatusRow, error) {
	var reported ReportWorkspaceAgentSubagentExecutionStatusRow
	err := q.InTx(func(tx Store) error {
		execution, err := tx.GetWorkspaceAgentSubagentExecutionForAcquisition(ctx, GetWorkspaceAgentSubagentExecutionForAcquisitionParams{
			WorkspaceBuildID: arg.WorkspaceBuildID,
			DeclarationID:    arg.DeclarationID,
			ParentAgentID:    arg.ParentAgentID,
		})
		if err != nil {
			return xerrors.Errorf("get subagent execution for report: %w", err)
		}

		if err := tx.AcquireWorkspaceBuildPublicationLock(ctx, execution.WorkspaceID); err != nil {
			return xerrors.Errorf("acquire workspace build publication lock: %w", err)
		}

		locked, err := tx.LockWorkspaceAgentSubagentExecutionStatusForReport(ctx, LockWorkspaceAgentSubagentExecutionStatusForReportParams{
			WorkspaceBuildID:   arg.WorkspaceBuildID,
			DeclarationID:      arg.DeclarationID,
			AcquisitionVersion: arg.AcquisitionVersion,
		})
		if err != nil {
			return xerrors.Errorf("lock subagent execution status: %w", err)
		}
		// The status is read under the row lock, so no concurrent acquisition or
		// report can move it between this check and the mutation.
		if !slices.Contains(reportableSubagentExecutionStatuses[SubagentExecutionStatus(locked.Status)], arg.Status) {
			return sql.ErrNoRows
		}

		latest, err := tx.GetLatestWorkspaceBuildGeneration(ctx, execution.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get latest workspace build generation: %w", err)
		}
		switch {
		case latest.ID == arg.WorkspaceBuildID:
			// The reported build is still the workspace's current generation.
			if latest.Transition != WorkspaceTransitionStart {
				return sql.ErrNoRows
			}
		case latest.Transition == WorkspaceTransitionStop:
			// The workspace has been stopped. The launcher of the generation the
			// stop replaced is being torn down by it, so it may still record
			// that it is going away, and nothing else.
			if !slices.Contains(stoppedGenerationSubagentExecutionStatuses, arg.Status) {
				return sql.ErrNoRows
			}
			preceding, err := tx.GetPrecedingStartWorkspaceBuildGeneration(ctx, latest.ID)
			if err != nil {
				return xerrors.Errorf("get preceding start workspace build generation: %w", err)
			}
			// Only the generation the stop immediately replaced qualifies. An
			// older one was already superseded by a newer start.
			if preceding != arg.WorkspaceBuildID {
				return sql.ErrNoRows
			}
		default:
			// A newer start build has replaced the reported generation.
			return sql.ErrNoRows
		}

		parent, err := tx.GetWorkspaceAgentSubagentExecutionAcquisitionParent(ctx, GetWorkspaceAgentSubagentExecutionAcquisitionParentParams{
			ParentAgentID:    arg.ParentAgentID,
			WorkspaceBuildID: arg.WorkspaceBuildID,
		})
		if err != nil {
			return xerrors.Errorf("get subagent execution parent: %w", err)
		}

		if _, err := tx.LockWorkspaceAgentSubagentExecutionChildForReport(ctx, LockWorkspaceAgentSubagentExecutionChildForReportParams{
			ChildAgentID:  execution.ChildAgentID,
			ParentAgentID: uuid.NullUUID{UUID: parent.ID, Valid: true},
			ResourceID:    parent.ResourceID,
		}); err != nil {
			return xerrors.Errorf("lock subagent execution child: %w", err)
		}

		reported, err = tx.MarkWorkspaceAgentSubagentExecutionReported(ctx, MarkWorkspaceAgentSubagentExecutionReportedParams{
			Status:             string(arg.Status),
			Now:                arg.Now,
			LastError:          arg.LastError,
			WorkspaceBuildID:   arg.WorkspaceBuildID,
			DeclarationID:      arg.DeclarationID,
			AcquisitionVersion: arg.AcquisitionVersion,
		})
		if err != nil {
			return xerrors.Errorf("mark subagent execution reported: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return ReportWorkspaceAgentSubagentExecutionStatusRow{}, err
	}
	return reported, nil
}
