package database

import (
	"context"

	"golang.org/x/xerrors"
)

// DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwnedParams keeps
// the public Store parameters stable while the soft-delete statement remains an
// internal generated query.
type DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwnedParams = SoftDeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwnedParams

// DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwned soft-deletes
// one exact child agent and removes its context state in the same transaction.
func (q *sqlQuerier) DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwned(ctx context.Context, arg DeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwnedParams) (int64, error) {
	var count int64
	err := q.InTx(func(tx Store) error {
		var err error
		count, err = tx.SoftDeleteWorkspaceAgentChildByIDAndParentIDExcludingExecutionOwned(ctx, arg)
		if err != nil {
			return xerrors.Errorf("soft delete workspace agent child: %w", err)
		}
		if count == 0 {
			return nil
		}

		if err := tx.DeleteWorkspaceAgentContextResourcesByAgentID(ctx, arg.ID); err != nil {
			return xerrors.Errorf("delete workspace agent context resources: %w", err)
		}
		if err := tx.DeleteWorkspaceAgentContextSnapshotByAgentID(ctx, arg.ID); err != nil {
			return xerrors.Errorf("delete workspace agent context snapshot: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return 0, err
	}
	return count, nil
}
