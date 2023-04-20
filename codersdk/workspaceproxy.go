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

type WorkspaceProxy struct {
	ID   uuid.UUID `db:"id" json:"id" format:"uuid" table:"id"`
	Name string    `db:"name" json:"name" table:"name,default_sort"`
	Icon string    `db:"icon" json:"icon" table:"icon"`
	// Full url including scheme of the proxy api url: https://us.example.com
	URL string `db:"url" json:"url" table:"url"`
	// WildcardHostname with the wildcard for subdomain based app hosting: *.us.example.com
	WildcardHostname string    `db:"wildcard_hostname" json:"wildcard_hostname" table:"wildcard_hostname"`
	CreatedAt        time.Time `db:"created_at" json:"created_at" format:"date-time" table:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at" format:"date-time" table:"updated_at"`
	Deleted          bool      `db:"deleted" json:"deleted" table:"deleted"`
}

type CreateWorkspaceProxyRequest struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Icon             string `json:"icon"`
	URL              string `json:"url"`
	WildcardHostname string `json:"wildcard_hostname"`
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
