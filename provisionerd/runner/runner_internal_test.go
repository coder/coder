package runner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestFailedWorkspaceBuildfDoesNotInferQuotaErrorCode(t *testing.T) {
	t.Parallel()

	r := &Runner{job: &proto.AcquiredJob{JobId: "job"}}
	failed := r.failedWorkspaceBuildf(
		"provider failed: insufficient quota in us-east1",
	)

	require.Empty(t, failed.ErrorCode)
}
