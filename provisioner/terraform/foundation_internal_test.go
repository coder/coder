package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestFoundationPlanFailureContext(t *testing.T) {
	t.Parallel()
	for _, snapshot := range []string{"", "apply-run"} {
		t.Run(snapshot, func(t *testing.T) {
			t.Parallel()
			message := foundationPlanFailureContext(&proto.Metadata{
				FoundationId: "foundation-id", FoundationName: "shared-services",
				TemplateName: "developer-workspace", FoundationSnapshotId: snapshot,
				TemplateOutputs: `{"password":"private-marker"}`,
			})
			require.Contains(t, message, "foundation-id")
			require.Contains(t, message, "shared-services")
			require.Contains(t, message, "developer-workspace")
			require.NotContains(t, message, "private-marker")
			if snapshot == "" {
				require.Contains(t, message, "none available")
			} else {
				require.Contains(t, message, snapshot)
			}
		})
	}
}
