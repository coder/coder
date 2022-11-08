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

type WorkspaceAppSharingLevel string

const (
	WorkspaceAppSharingLevelOwner         WorkspaceAppSharingLevel = "owner"
	WorkspaceAppSharingLevelAuthenticated WorkspaceAppSharingLevel = "authenticated"
	WorkspaceAppSharingLevelPublic        WorkspaceAppSharingLevel = "public"
)

type WorkspaceApp struct {
	ID uuid.UUID `json:"id"`
	// Slug is a unique identifier within the agent.
	Slug string `json:"slug"`
	// DisplayName is a friendly name for the app.
	DisplayName string `json:"display_name"`
	Command     string `json:"command,omitempty"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon,omitempty"`
	// Subdomain denotes whether the app should be accessed via a path on the
	// `coder server` or via a hostname-based dev URL. If this is set to true
	// and there is no app wildcard configured on the server, the app will not
	// be accessible in the UI.
	Subdomain    bool                     `json:"subdomain"`
	SharingLevel WorkspaceAppSharingLevel `json:"sharing_level"`
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
	Healths map[uuid.UUID]WorkspaceAppHealth
}
