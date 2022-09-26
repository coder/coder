package codersdk

import (
	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppHealthDisabled     WorkspaceAppHealth = "disabled"
	WorkspaceAppHealthInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppHealthUnhealthy    WorkspaceAppHealth = "unhealthy"
)

type WorkspaceApp struct {
	ID uuid.UUID `json:"id"`
	// Name is a unique identifier attached to an agent.
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon,omitempty"`
	// Healthcheck specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck"`
	Health      WorkspaceAppHealth `json:"health"`
}

type Healthcheck struct {
	// URL specifies the url to check for the app health.
	URL string `json:"url"`
	// Interval specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Threshold specifies the number of consecutive failed health checks before returning "unhealthy".
	Threshold int32 `json:"threshold"`
}

// @typescript-ignore PostWorkspaceAppHealthsRequest
type PostWorkspaceAppHealthsRequest struct {
	// Healths is a map of the workspace app name and the health of the app.
	Healths map[string]WorkspaceAppHealth
}
