package database

import (
	"context"
	"database/sql"
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
