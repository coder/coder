package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

// WorkspaceSkillMetadata represents a workspace skill without its raw Markdown content.
type WorkspaceSkillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func workspaceSkillsPath(workspaceID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/workspaces/%s/skills", url.PathEscape(workspaceID.String()))
}

// WorkspaceSkills lists workspace skill metadata for the specified workspace.
func (c *ExperimentalClient) WorkspaceSkills(ctx context.Context, workspaceID uuid.UUID) ([]WorkspaceSkillMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, workspaceSkillsPath(workspaceID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var skills []WorkspaceSkillMetadata
	return skills, json.NewDecoder(res.Body).Decode(&skills)
}
