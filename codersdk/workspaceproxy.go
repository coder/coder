package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"
)

type ProxyHealthStatus string

const (
	// ProxyReachable means the proxy access url is reachable and returns a healthy
	// status code.
	ProxyReachable ProxyHealthStatus = "reachable"
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
	Status ProxyHealthStatus `json:"status" table:"status"`
	// Report provides more information about the health of the workspace proxy.
	Report    ProxyHealthReport `json:"report,omitempty" table:"report"`
	CheckedAt time.Time         `json:"checked_at" table:"checked_at" format:"date-time"`
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
	ID   uuid.UUID `json:"id" format:"uuid" table:"id"`
	Name string    `json:"name" table:"name,default_sort"`
	Icon string    `json:"icon" table:"icon"`
	// Full url including scheme of the proxy api url: https://us.example.com
	URL string `json:"url" table:"url"`
	// WildcardHostname with the wildcard for subdomain based app hosting: *.us.example.com
	WildcardHostname string    `json:"wildcard_hostname" table:"wildcard_hostname"`
	CreatedAt        time.Time `json:"created_at" format:"date-time" table:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time" table:"updated_at"`
	Deleted          bool      `json:"deleted" table:"deleted"`

	// Status is the latest status check of the proxy. This will be empty for deleted
	// proxies. This value can be used to determine if a workspace proxy is healthy
	// and ready to use.
	Status WorkspaceProxyStatus `json:"status,omitempty" table:"status"`
}

type CreateWorkspaceProxyRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}

type CreateWorkspaceProxyResponse struct {
	Proxy WorkspaceProxy `json:"proxy" table:"proxy,recursive"`
	// The recursive table sort is not working very well.
	ProxyToken string `json:"proxy_token" table:"proxy token,default_sort"`
}

func (c *Client) CreateWorkspaceProxy(ctx context.Context, req CreateWorkspaceProxyRequest) (CreateWorkspaceProxyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies",
		req,
	)
	if err != nil {
		return CreateWorkspaceProxyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return CreateWorkspaceProxyResponse{}, ReadBodyAsError(res)
	}
	var resp CreateWorkspaceProxyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) WorkspaceProxies(ctx context.Context) ([]WorkspaceProxy, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/workspaceproxies",
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var proxies []WorkspaceProxy
	return proxies, json.NewDecoder(res.Body).Decode(&proxies)
}

func (c *Client) DeleteWorkspaceProxyByName(ctx context.Context, name string) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/workspaceproxies/%s", name),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
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

type RegionsResponse struct {
	Regions []Region `json:"regions"`
}

type Region struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	IconURL     string    `json:"icon_url"`
	Healthy     bool      `json:"healthy"`

	// PathAppURL is the URL to the base path for path apps. Optional
	// unless wildcard_hostname is set.
	// E.g. https://us.example.com
	PathAppURL string `json:"path_app_url"`

	// WildcardHostname is the wildcard hostname for subdomain apps.
	// E.g. *.us.example.com
	// E.g. *--suffix.au.example.com
	// Optional. Does not need to be on the same domain as PathAppURL.
	WildcardHostname string `json:"wildcard_hostname"`
}

func (c *Client) Regions(ctx context.Context) ([]Region, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/regions",
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var regions RegionsResponse
	return regions.Regions, json.NewDecoder(res.Body).Decode(&regions)
}
