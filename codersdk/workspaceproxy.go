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

type CreateWorkspaceProxyRequest struct {
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	URL         string `json:"url"`
	WildcardURL string `json:"wildcard_url"`
}

type WorkspaceProxy struct {
	ID             uuid.UUID `db:"id" json:"id"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id"`
	Name           string    `db:"name" json:"name"`
	Icon           string    `db:"icon" json:"icon"`
	// Full url including scheme of the proxy api url: https://us.example.com
	Url string `db:"url" json:"url"`
	// URL with the wildcard for subdomain based app hosting: https://*.us.example.com
	WildcardUrl string    `db:"wildcard_url" json:"wildcard_url"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
	Deleted     bool      `db:"deleted" json:"deleted"`
}

func (c *Client) CreateWorkspaceProxy(ctx context.Context, orgID uuid.UUID, req CreateWorkspaceProxyRequest) (WorkspaceProxy, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/workspaceproxies", orgID.String()),
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

func (c *Client) WorkspaceProxiesByOrganization(ctx context.Context, orgID uuid.UUID) ([]WorkspaceProxy, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/workspaceproxies", orgID.String()),
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
