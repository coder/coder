package codersdk

import (
	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppUnhealthy    WorkspaceAppHealth = "unhealthy"
)

type WorkspaceApp struct {
	ID uuid.UUID `json:"id"`
	// Name is a unique identifier attached to an agent.
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon   string             `json:"icon,omitempty"`
	Status WorkspaceAppHealth `json:"health"`
}

type PostWorkspaceAppHealthsRequest struct {
	// Healths is a map of the workspace app name and the status of the app.
	Healths map[string]WorkspaceAppHealth
}
