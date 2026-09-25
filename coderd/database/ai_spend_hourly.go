package database

import (
	"context"

	"golang.org/x/xerrors"
)

type IncrementAIBridgeTokenUsageHourlyParams = IncrementAIBridgeTokenUsageHourlyLockedParams

// IncrementAIBridgeTokenUsageHourly serializes writes to the same user, group,
// and hour before reading the dimension row under a fresh READ COMMITTED snapshot.
func (q *sqlQuerier) IncrementAIBridgeTokenUsageHourly(ctx context.Context, arg IncrementAIBridgeTokenUsageHourlyParams) error {
	return q.InTx(func(tx Store) error {
		if err := tx.LockAIBridgeHourlyBucket(ctx, LockAIBridgeHourlyBucketParams{
			EffectiveGroupID: arg.EffectiveGroupID,
			InitiatorID:      arg.InitiatorID,
			CreatedAt:        arg.CreatedAt,
		}); err != nil {
			return xerrors.Errorf("lock hourly usage bucket: %w", err)
		}
		if err := tx.IncrementAIBridgeTokenUsageHourlyLocked(ctx, arg); err != nil {
			return xerrors.Errorf("increment hourly usage: %w", err)
		}
		return nil
	}, nil)
}
