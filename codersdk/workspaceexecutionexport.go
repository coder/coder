package codersdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// ExportWorkspaceExecutionRequest names the last observed open session revision.
// Export ends command admission for this session without deleting its workspace.
type ExportWorkspaceExecutionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

// ExportWorkspaceExecution preserves the declared output paths as one immutable
// generation. Retry the same expected revision after an interrupted response.
func (c *Client) ExportWorkspaceExecution(ctx context.Context, organization, session uuid.UUID, request ExportWorkspaceExecutionRequest) ([]WorkspaceExecutionArtifact, error) {
	response, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/export", organization, session), request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(response)
	}
	var result []WorkspaceExecutionArtifact
	err = ReadBodyAsJSON(response, &result)
	return result, err
}
