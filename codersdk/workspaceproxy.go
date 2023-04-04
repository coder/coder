package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"
)

type CreateWorkspaceProxyRequest struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Icon             string `json:"icon"`
	URL              string `json:"url"`
	WildcardHostname string `json:"wildcard_hostname"`
}

type WorkspaceProxy struct {
	ID             uuid.UUID `db:"id" json:"id" format:"uuid"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id" format:"uuid"`
	Name           string    `db:"name" json:"name"`
	Icon           string    `db:"icon" json:"icon"`
	// Full url including scheme of the proxy api url: https://us.example.com
	URL string `db:"url" json:"url"`
	// WildcardHostname with the wildcard for subdomain based app hosting: *.us.example.com
	WildcardHostname string    `db:"wildcard_hostname" json:"wildcard_hostname"`
	CreatedAt        time.Time `db:"created_at" json:"created_at" format:"date-time"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at" format:"date-time"`
	Deleted          bool      `db:"deleted" json:"deleted"`
}

func (c *Client) CreateWorkspaceProxy(ctx context.Context, req CreateWorkspaceProxyRequest) (WorkspaceProxy, error) {
	res, err := c.Request(ctx, http.MethodPost,
		"/api/v2/workspaceproxies",
		req,
	)
	if err != nil {
		return WorkspaceProxy{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return WorkspaceProxy{}, ReadBodyAsError(res)
	}
	var resp WorkspaceProxy
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) WorkspaceProxiesByOrganization(ctx context.Context) ([]WorkspaceProxy, error) {
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
