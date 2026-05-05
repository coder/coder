package runner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionerd/proto"
)

func TestFailedJobfSetsInsufficientQuotaErrorCode(t *testing.T) {
	t.Parallel()

	r := &Runner{
		job: &proto.AcquiredJob{JobId: "test-job"},
	}

	job := r.failedJobf("workspace build failed: insufficient quota")
	require.Equal(t, InsufficientQuotaErrorCode, job.ErrorCode)
	require.Equal(t, "workspace build failed: insufficient quota", job.Error)
}
