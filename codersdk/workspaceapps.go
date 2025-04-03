package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppHealthDisabled     WorkspaceAppHealth = "disabled"
	WorkspaceAppHealthInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppHealthUnhealthy    WorkspaceAppHealth = "unhealthy"
)

type WorkspaceAppStatusState string

const (
	WorkspaceAppStatusStateWorking  WorkspaceAppStatusState = "working"
	WorkspaceAppStatusStateComplete WorkspaceAppStatusState = "complete"
	WorkspaceAppStatusStateFailure  WorkspaceAppStatusState = "failure"
)

var MapWorkspaceAppHealths = map[WorkspaceAppHealth]struct{}{
	WorkspaceAppHealthDisabled:     {},
	WorkspaceAppHealthInitializing: {},
	WorkspaceAppHealthHealthy:      {},
	WorkspaceAppHealthUnhealthy:    {},
}

type WorkspaceAppSharingLevel string

const (
	WorkspaceAppSharingLevelOwner         WorkspaceAppSharingLevel = "owner"
	WorkspaceAppSharingLevelAuthenticated WorkspaceAppSharingLevel = "authenticated"
	WorkspaceAppSharingLevelPublic        WorkspaceAppSharingLevel = "public"
)

var MapWorkspaceAppSharingLevels = map[WorkspaceAppSharingLevel]struct{}{
	WorkspaceAppSharingLevelOwner:         {},
	WorkspaceAppSharingLevelAuthenticated: {},
	WorkspaceAppSharingLevelPublic:        {},
}

type WorkspaceAppOpenIn string

const (
	WorkspaceAppOpenInSlimWindow WorkspaceAppOpenIn = "slim-window"
	WorkspaceAppOpenInTab        WorkspaceAppOpenIn = "tab"
)

var MapWorkspaceAppOpenIns = map[WorkspaceAppOpenIn]struct{}{
	WorkspaceAppOpenInSlimWindow: {},
	WorkspaceAppOpenInTab:        {},
}

type WorkspaceApp struct {
	ID uuid.UUID `json:"id" format:"uuid"`
	// URL is the address being proxied to inside the workspace.
	// If external is specified, this will be opened on the client.
	URL string `json:"url"`
	// External specifies whether the URL should be opened externally on
	// the client or not.
	External bool `json:"external"`
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
	Subdomain bool `json:"subdomain"`
	// SubdomainName is the application domain exposed on the `coder server`.
	SubdomainName string                   `json:"subdomain_name,omitempty"`
	SharingLevel  WorkspaceAppSharingLevel `json:"sharing_level" enums:"owner,authenticated,public"`
	// Healthcheck specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck"`
	Health      WorkspaceAppHealth `json:"health"`
	Hidden      bool               `json:"hidden"`
	OpenIn      WorkspaceAppOpenIn `json:"open_in"`

	// Statuses is a list of statuses for the app.
	Statuses []WorkspaceAppStatus `json:"statuses"`
}

type Healthcheck struct {
	// URL specifies the endpoint to check for the app health.
	URL string `json:"url"`
	// Interval specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Threshold specifies the number of consecutive failed health checks before returning "unhealthy".
	Threshold int32 `json:"threshold"`
}

type WorkspaceAppStatus struct {
	ID                 uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt          time.Time               `json:"created_at" format:"date-time"`
	WorkspaceID        uuid.UUID               `json:"workspace_id" format:"uuid"`
	AgentID            uuid.UUID               `json:"agent_id" format:"uuid"`
	AppID              uuid.UUID               `json:"app_id" format:"uuid"`
	State              WorkspaceAppStatusState `json:"state"`
	NeedsUserAttention bool                    `json:"needs_user_attention"`
	Message            string                  `json:"message"`
	// URI is the URI of the resource that the status is for.
	// e.g. https://github.com/org/repo/pull/123
	// e.g. file:///path/to/file
	URI string `json:"uri"`
	// Icon is an external URL to an icon that will be rendered in the UI.
	Icon string `json:"icon"`
}
