package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type GroupSource string

const (
	GroupSourceUser GroupSource = "user"
	GroupSourceOIDC GroupSource = "oidc"
)

type CreateGroupRequest struct {
	Name           string `json:"name" validate:"required,group_name"`
	DisplayName    string `json:"display_name" validate:"omitempty,group_display_name"`
	AvatarURL      string `json:"avatar_url"`
	QuotaAllowance int    `json:"quota_allowance"`
}

type Group struct {
	ID             uuid.UUID     `json:"id" format:"uuid"`
	Name           string        `json:"name"`
	DisplayName    string        `json:"display_name"`
	OrganizationID uuid.UUID     `json:"organization_id" format:"uuid"`
	Members        []ReducedUser `json:"members"`
	// How many members are in this group. Shows the total count,
	// even if the user is not authorized to read group member details.
	// May be greater than `len(Group.Members)`.
	TotalMemberCount int         `json:"total_member_count"`
	AvatarURL        string      `json:"avatar_url"`
	QuotaAllowance   int         `json:"quota_allowance"`
	Source           GroupSource `json:"source"`
}

type GroupWithOrganizationInfo struct {
	Group
	OrganizationName        string `json:"organization_name"`
	OrganizationDisplayName string `json:"organization_display_name"`
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

// GroupsByOrganization
// Deprecated: use Groups with GroupArguments instead.
func (c *Client) GroupsByOrganization(ctx context.Context, orgID uuid.UUID) ([]Group, error) {
	return c.Groups(ctx, GroupArguments{Organization: orgID.String()})
}

type GroupArguments struct {
	// Organization can be an org UUID or name
	Organization string
	// HasMember can be a user uuid or username
	HasMember string
}

func (c *Client) Groups(ctx context.Context, args GroupArguments) ([]Group, error) {
	qp := url.Values{}
	if args.Organization != "" {
		qp.Set("organization", args.Organization)
	}
	if args.HasMember != "" {
		qp.Set("has_member", args.HasMember)
	}

	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups?%s", qp.Encode()),
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
	Name           string   `json:"name" validate:"omitempty,group_name"`
	DisplayName    *string  `json:"display_name" validate:"omitempty,group_display_name"`
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
