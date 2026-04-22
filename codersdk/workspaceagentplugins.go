package codersdk

import "github.com/google/uuid"

// WorkspaceAgentPlugin represents a UI plugin attached to a workspace agent.
// Plugins are rendered as iframe tabs in the agents UI RightPanel.
type WorkspaceAgentPlugin struct {
	ID           uuid.UUID `json:"id" format:"uuid"`
	Slug         string    `json:"slug"`
	DisplayName  string    `json:"display_name"`
	Icon         string    `json:"icon"`
	URL          string    `json:"url"`
	BackendEntry string    `json:"backend_entry,omitempty"`
}
