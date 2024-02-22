package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type GroupSource string

const (
	GroupSourceUser GroupSource = "user"
	GroupSourceOIDC GroupSource = "oidc"
)

type CreateGroupRequest struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	AvatarURL      string `json:"avatar_url"`
	QuotaAllowance int    `json:"quota_allowance"`
}

type Group struct {
	ID             uuid.UUID     `json:"id" format:"uuid"`
	Name           string        `json:"name"`
	DisplayName    string        `json:"display_name"`
	OrganizationID uuid.UUID     `json:"organization_id" format:"uuid"`
	Members        []ReducedUser `json:"members"`
	AvatarURL      string        `json:"avatar_url"`
	QuotaAllowance int           `json:"quota_allowance"`
	Source         GroupSource   `json:"source"`
}

func (g Group) IsEveryone() bool {
	return g.ID == g.OrganizationID
}

func (c *Client) CreateGroup(ctx context.Context, orgID uuid.UUID, req CreateGroupRequest) (Group, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/groups", orgID.String()),
		req,
	)
	if err != nil {
		return Group{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Group{}, ReadBodyAsError(res)
	}
	var resp Group
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GroupsByOrganization(ctx context.Context, orgID uuid.UUID) ([]Group, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/groups", orgID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var groups []Group
	return groups, json.NewDecoder(res.Body).Decode(&groups)
}

func (c *Client) GroupByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (Group, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/groups/%s", orgID.String(), name),
		nil,
	)
	if err != nil {
		return Group{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Group{}, ReadBodyAsError(res)
	}
	var resp Group
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) Group(ctx context.Context, group uuid.UUID) (Group, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups/%s", group.String()),
		nil,
	)
	if err != nil {
		return Group{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Group{}, ReadBodyAsError(res)
	}
	var resp Group
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type PatchGroupRequest struct {
	AddUsers       []string `json:"add_users"`
	RemoveUsers    []string `json:"remove_users"`
	Name           string   `json:"name"`
	DisplayName    *string  `json:"display_name"`
	AvatarURL      *string  `json:"avatar_url"`
	QuotaAllowance *int     `json:"quota_allowance"`
}

func (c *Client) PatchGroup(ctx context.Context, group uuid.UUID, req PatchGroupRequest) (Group, error) {
	res, err := c.Request(ctx, http.MethodPatch,
		fmt.Sprintf("/api/v2/groups/%s", group.String()),
		req,
	)
	if err != nil {
		return Group{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Group{}, ReadBodyAsError(res)
	}
	var resp Group
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) DeleteGroup(ctx context.Context, group uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/groups/%s", group.String()),
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
