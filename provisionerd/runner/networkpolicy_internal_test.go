package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestNetworkPolicyForcedFailureStateDisposition(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		applyStarted bool
		want         proto.StateDisposition
	}{
		{name: "BeforeApply", want: proto.StateDisposition_UNCHANGED},
		{name: "AfterApply", applyStarted: true, want: proto.StateDisposition_UNKNOWN},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updater := &forcedPolicyUpdater{}
			r := New(t.Context(), &proto.AcquiredJob{
				JobId: uuid.NewString(),
				Type: &proto.AcquiredJob_NetworkPolicy_{NetworkPolicy: &proto.AcquiredJob_NetworkPolicy{
					Operation: proto.NetworkPolicyOperation_APPLY,
				}},
			}, Options{Updater: updater})
			// A heartbeat failure can be constructed before apply starts,
			// then delivered after it starts. Fail must use the latest phase.
			failed := r.failedJobf("lost server connection")
			r.networkPolicyApplyStarted.Store(tt.applyStarted)
			require.NoError(t, r.Fail(context.Background(), failed))
			require.NotNil(t, updater.failed.GetNetworkPolicy())
			require.Equal(t, tt.want, updater.failed.GetNetworkPolicy().StateDisposition)
			require.Empty(t, updater.failed.GetNetworkPolicy().State)
			require.ErrorIs(t, r.notCanceled.Err(), context.Canceled)
		})
	}
}

type forcedPolicyUpdater struct {
	JobUpdater
	failed *proto.FailedJob
}

func (u *forcedPolicyUpdater) FailJob(_ context.Context, job *proto.FailedJob) error {
	u.failed = job
	return nil
}
