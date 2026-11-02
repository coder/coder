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
	WorkspaceAppStatusStateIdle     WorkspaceAppStatusState = "idle"
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
	WorkspaceAppSharingLevelOrganization  WorkspaceAppSharingLevel = "organization"
	WorkspaceAppSharingLevelPublic        WorkspaceAppSharingLevel = "public"
)

var MapWorkspaceAppSharingLevels = map[WorkspaceAppSharingLevel]struct{}{
	WorkspaceAppSharingLevelOwner:         {},
	WorkspaceAppSharingLevelAuthenticated: {},
	WorkspaceAppSharingLevelOrganization:  {},
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
	// The address being proxied to inside the workspace. If `external` is
	// specified, this will be opened on the client.
	URL string `json:"url,omitempty"`
	// Whether the `url` should be opened externally on the client or not.
	External bool `json:"external"`
	// A unique identifier within the agent.
	Slug string `json:"slug"`
	// A friendly name for the app.
	DisplayName string `json:"display_name,omitempty"`
	Command     string `json:"command,omitempty"`
	// A relative path or external `url` that specifies an icon to be displayed in
	// the dashboard.
	Icon string `json:"icon,omitempty"`
	// Denotes whether the app should be accessed via a path on the `coder server`
	// or via a hostname-based dev `url`. If this is set to true and there is no
	// app wildcard configured on the server, the app will not be accessible in the
	// UI.
	Subdomain bool `json:"subdomain"`
	// The application domain exposed on the `coder server`.
	SubdomainName string                   `json:"subdomain_name,omitempty"`
	SharingLevel  WorkspaceAppSharingLevel `json:"sharing_level" enums:"owner,authenticated,organization,public"`
	// Specifies the configuration for checking app health.
	Healthcheck Healthcheck        `json:"healthcheck,omitempty"`
	Health      WorkspaceAppHealth `json:"health"`
	Group       string             `json:"group,omitempty"`
	Hidden      bool               `json:"hidden"`
	OpenIn      WorkspaceAppOpenIn `json:"open_in"`
	// An optional markdown supported field that is displayed when hovering over
	// workspace apps in the UI.
	Tooltip string `json:"tooltip,omitempty"`

	// A list of statuses for the app.
	Statuses []WorkspaceAppStatus `json:"statuses"`
}

type Healthcheck struct {
	// Specifies the endpoint to check for the app health.
	URL string `json:"url"`
	// Specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Specifies the number of consecutive failed health checks before returning
	// "unhealthy".
	Threshold int32 `json:"threshold"`
}

type WorkspaceAppStatus struct {
	ID          uuid.UUID               `json:"id" format:"uuid"`
	CreatedAt   time.Time               `json:"created_at" format:"date-time"`
	WorkspaceID uuid.UUID               `json:"workspace_id" format:"uuid"`
	AgentID     uuid.UUID               `json:"agent_id" format:"uuid"`
	AppID       uuid.UUID               `json:"app_id" format:"uuid"`
	State       WorkspaceAppStatusState `json:"state"`
	Message     string                  `json:"message"`
	// The `uri` of the resource that the status is for. e.g.
	// https://github.com/org/repo/pull/123 e.g. file:///path/to/file
	URI string `json:"uri"`

	// Deprecated: This field is unused and will be removed in a future version.
	// Icon is an external URL to an icon that will be rendered in the UI.
	Icon string `json:"icon"`
	// Deprecated: This field is unused and will be removed in a future version.
	// NeedsUserAttention specifies whether the status needs user attention.
	NeedsUserAttention bool `json:"needs_user_attention"`
}
