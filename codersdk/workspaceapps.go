package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type WorkspaceAppHealth string

const (
	WorkspaceAppHealthDisabled     WorkspaceAppHealth = "disabled"
	WorkspaceAppHealthInitializing WorkspaceAppHealth = "initializing"
	WorkspaceAppHealthHealthy      WorkspaceAppHealth = "healthy"
	WorkspaceAppHealthUnhealthy    WorkspaceAppHealth = "unhealthy"
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
}

type Healthcheck struct {
	// URL specifies the endpoint to check for the app health.
	URL string `json:"url"`
	// Interval specifies the seconds between each health check.
	Interval int32 `json:"interval"`
	// Threshold specifies the number of consecutive failed health checks before returning "unhealthy".
	Threshold int32 `json:"threshold"`
}

// WorkspaceAgentAppURL contains URL to access a workspace app
type WorkspaceAgentAppURL struct {
	URL string `json:"url"`
}

// WorkspaceAgentApps returns a list of apps for the given workspace agent.
func (c *Client) WorkspaceAgentApps(ctx context.Context, agentID uuid.UUID) ([]WorkspaceApp, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/apps", agentID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var apps []WorkspaceApp
	err = json.NewDecoder(res.Body).Decode(&apps)
	if err != nil {
		return nil, err
	}

	return apps, nil
}

// WorkspaceAgentAppURL returns the URL for a specific app on a workspace agent.
func (c *Client) WorkspaceAgentAppURL(ctx context.Context, agentID uuid.UUID, appSlug string) (WorkspaceAgentAppURL, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceagents/%s/apps/%s/url", agentID, appSlug), nil)
	if err != nil {
		return WorkspaceAgentAppURL{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAppURL{}, ReadBodyAsError(res)
	}

	var appURL WorkspaceAgentAppURL
	err = json.NewDecoder(res.Body).Decode(&appURL)
	if err != nil {
		return WorkspaceAgentAppURL{}, err
	}

	return appURL, nil
}
