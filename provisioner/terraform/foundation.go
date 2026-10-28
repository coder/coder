package terraform

import (
	"fmt"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func foundationPlanFailureContext(metadata *proto.Metadata) string {
	snapshot := metadata.GetFoundationSnapshotId()
	if snapshot == "" {
		snapshot = "none available"
	}
	return fmt.Sprintf("Foundation %q (%s), template %q, output snapshot: %s. If the Terraform diagnostics above identify a missing required output, apply compatible Foundation configuration or update the consuming template. Optional accesses may use try(...).", metadata.GetFoundationName(), metadata.GetFoundationId(), metadata.GetTemplateName(), snapshot)
}
