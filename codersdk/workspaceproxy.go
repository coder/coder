package codersdk
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/google/uuid"
)
type ProxyHealthStatus string
const (
	// ProxyHealthy means the proxy access url is reachable and returns a healthy
	// status code.
	ProxyHealthy ProxyHealthStatus = "ok"
	// ProxyUnreachable means the proxy access url is not responding.
	ProxyUnreachable ProxyHealthStatus = "unreachable"
	// ProxyUnhealthy means the proxy access url is responding, but there is some
	// problem with the proxy. This problem may or may not be preventing functionality.
	ProxyUnhealthy ProxyHealthStatus = "unhealthy"
	// ProxyUnregistered means the proxy has not registered a url yet. This means
	// the proxy was created with the cli, but has not yet been started.
	ProxyUnregistered ProxyHealthStatus = "unregistered"
)
type WorkspaceProxyStatus struct {
	Status ProxyHealthStatus `json:"status" table:"status,default_sort"`
	// Report provides more information about the health of the workspace proxy.
	Report    ProxyHealthReport `json:"report,omitempty" table:"report"`
	CheckedAt time.Time         `json:"checked_at" table:"checked at" format:"date-time"`
}
// ProxyHealthReport is a report of the health of the workspace proxy.
// A healthy report will have no errors. Warnings are not fatal.
type ProxyHealthReport struct {
	// Errors are problems that prevent the workspace proxy from being healthy
	Errors []string `json:"errors"`
	// Warnings do not prevent the workspace proxy from being healthy, but
	// should be addressed.
	Warnings []string `json:"warnings"`
}
type WorkspaceProxy struct {
	// Extends Region with extra information
	Region      `table:"region,recursive_inline"`
	DerpEnabled bool `json:"derp_enabled" table:"derp enabled"`
	DerpOnly    bool `json:"derp_only" table:"derp only"`
	// Status is the latest status check of the proxy. This will be empty for deleted
	// proxies. This value can be used to determine if a workspace proxy is healthy
	// and ready to use.
	Status WorkspaceProxyStatus `json:"status,omitempty" table:"proxy,recursive"`
	CreatedAt time.Time `json:"created_at" format:"date-time" table:"created at"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time" table:"updated at"`
	Deleted   bool      `json:"deleted" table:"deleted"`
	Version   string    `json:"version" table:"version"`
}
type CreateWorkspaceProxyRequest struct {
	Name        string `json:"name" validate:"required"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}
type UpdateWorkspaceProxyResponse struct {
	Proxy      WorkspaceProxy `json:"proxy" table:"p,recursive_inline"`
	ProxyToken string         `json:"proxy_token" table:"proxy token"`
}
func (c *Client) CreateWorkspaceProxy(ctx context.Context, req CreateWorkspaceProxyRequest) (UpdateWorkspaceProxyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies",
		req,
	)
	if err != nil {
		return UpdateWorkspaceProxyResponse{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return UpdateWorkspaceProxyResponse{}, ReadBodyAsError(res)
	}
	var resp UpdateWorkspaceProxyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
func (c *Client) WorkspaceProxies(ctx context.Context) (RegionsResponse[WorkspaceProxy], error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/workspaceproxies",
		nil,
	)
	if err != nil {
		return RegionsResponse[WorkspaceProxy]{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RegionsResponse[WorkspaceProxy]{}, ReadBodyAsError(res)
	}
	var proxies RegionsResponse[WorkspaceProxy]
	return proxies, json.NewDecoder(res.Body).Decode(&proxies)
}
type PatchWorkspaceProxy struct {
	ID              uuid.UUID `json:"id" format:"uuid" validate:"required"`
	Name            string    `json:"name" validate:"required"`
	DisplayName     string    `json:"display_name" validate:"required"`
	Icon            string    `json:"icon" validate:"required"`
	RegenerateToken bool      `json:"regenerate_token"`
}
func (c *Client) PatchWorkspaceProxy(ctx context.Context, req PatchWorkspaceProxy) (UpdateWorkspaceProxyResponse, error) {
	res, err := c.Request(ctx, http.MethodPatch,
		fmt.Sprintf("/api/v2/workspaceproxies/%s", req.ID.String()),
		req,
	)
	if err != nil {
		return UpdateWorkspaceProxyResponse{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UpdateWorkspaceProxyResponse{}, ReadBodyAsError(res)
	}
	var resp UpdateWorkspaceProxyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
func (c *Client) DeleteWorkspaceProxyByName(ctx context.Context, name string) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/workspaceproxies/%s", name),
		nil,
	)
	if err != nil {
		return fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
func (c *Client) DeleteWorkspaceProxyByID(ctx context.Context, id uuid.UUID) error {
	return c.DeleteWorkspaceProxyByName(ctx, id.String())
}
func (c *Client) WorkspaceProxyByName(ctx context.Context, name string) (WorkspaceProxy, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/workspaceproxies/%s", name),
		nil,
	)
	if err != nil {
		return WorkspaceProxy{}, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceProxy{}, ReadBodyAsError(res)
	}
	var resp WorkspaceProxy
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
func (c *Client) WorkspaceProxyByID(ctx context.Context, id uuid.UUID) (WorkspaceProxy, error) {
	return c.WorkspaceProxyByName(ctx, id.String())
}
type RegionTypes interface {
	Region | WorkspaceProxy
}
type RegionsResponse[R RegionTypes] struct {
	Regions []R `json:"regions"`
}
type Region struct {
	ID          uuid.UUID `json:"id" format:"uuid" table:"id"`
	Name        string    `json:"name" table:"name,default_sort"`
	DisplayName string    `json:"display_name" table:"display name"`
	IconURL     string    `json:"icon_url" table:"icon url"`
	Healthy     bool      `json:"healthy" table:"healthy"`
	// PathAppURL is the URL to the base path for path apps. Optional
	// unless wildcard_hostname is set.
	// E.g. https://us.example.com
	PathAppURL string `json:"path_app_url" table:"url"`
	// WildcardHostname is the wildcard hostname for subdomain apps.
	// E.g. *.us.example.com
	// E.g. *--suffix.au.example.com
	// Optional. Does not need to be on the same domain as PathAppURL.
	WildcardHostname string `json:"wildcard_hostname" table:"wildcard hostname"`
}
func (c *Client) Regions(ctx context.Context) ([]Region, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/regions",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("make request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var regions RegionsResponse[Region]
	return regions.Regions, json.NewDecoder(res.Body).Decode(&regions)
}
