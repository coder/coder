package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	TotalMemberCount        int         `json:"total_member_count"`
	AvatarURL               string      `json:"avatar_url" format:"uri"`
	QuotaAllowance          int         `json:"quota_allowance"`
	Source                  GroupSource `json:"source"`
	OrganizationName        string      `json:"organization_name"`
	OrganizationDisplayName string      `json:"organization_display_name"`
}

type GroupMembersResponse struct {
	Users []ReducedUser `json:"users"`
	Count int           `json:"count"`
}

type PaginatedGroupsResponse struct {
	Groups []PaginatedGroup `json:"groups"`
	Count  int              `json:"count"`
}

// PaginatedGroup is a group summary returned by the paginated groups endpoint.
// It deliberately omits the member roster (which the endpoint does not return)
// and exposes only the total member count. Fetch the roster via the group
// members endpoint.
type PaginatedGroup struct {
	ID             uuid.UUID `json:"id" format:"uuid"`
	Name           string    `json:"name"`
	DisplayName    string    `json:"display_name"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	// TotalMemberCount is the number of members in the group, shown even when
	// the caller cannot read individual members. The roster itself is not
	// returned by this endpoint.
	TotalMemberCount        int         `json:"total_member_count"`
	AvatarURL               string      `json:"avatar_url" format:"uri"`
	QuotaAllowance          int         `json:"quota_allowance"`
	Source                  GroupSource `json:"source"`
	OrganizationName        string      `json:"organization_name"`
	OrganizationDisplayName string      `json:"organization_display_name"`
}

// PaginatedGroupsRequest are the filters for a paginated groups request.
// Groups only support free-text search, so unlike UsersRequest it exposes no
// key:value filters that the endpoint would reject.
type PaginatedGroupsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

func (req PaginatedGroupsRequest) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if req.SearchQuery != "" {
			q.Set("q", req.SearchQuery)
		}
		r.URL.RawQuery = q.Encode()
	}
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
	return resp, ReadBodyAsJSON(res, &resp)
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
	// GroupIDs is a list of group UUIDs to filter by.
	// If not set, all groups will be returned.
	GroupIDs []uuid.UUID
}

func (c *Client) Groups(ctx context.Context, args GroupArguments) ([]Group, error) {
	qp := url.Values{}
	if args.Organization != "" {
		qp.Set("organization", args.Organization)
	}
	if args.HasMember != "" {
		qp.Set("has_member", args.HasMember)
	}
	if len(args.GroupIDs) > 0 {
		idStrs := make([]string, 0, len(args.GroupIDs))
		for _, id := range args.GroupIDs {
			idStrs = append(idStrs, id.String())
		}
		qp.Set("group_ids", strings.Join(idStrs, ","))
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
	return groups, ReadBodyAsJSON(res, &groups)
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
	return resp, ReadBodyAsJSON(res, &resp)
}

// OrganizationGroupsPaginated lists filtered and paginated groups in an
// organization. Unlike Groups (GET /groups), which authorizes each group
// individually via its ACL, this endpoint requires organization-wide group
// read permission and does no per-group filtering. It is therefore not a
// drop-in replacement for Groups: callers without org-wide group read will
// receive an error rather than a filtered subset.
func (c *Client) OrganizationGroupsPaginated(ctx context.Context, orgID uuid.UUID, req PaginatedGroupsRequest) (PaginatedGroupsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/paginated-groups", orgID.String()),
		nil,
		req.Pagination.asRequestOption(),
		req.asRequestOption(),
	)
	if err != nil {
		return PaginatedGroupsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return PaginatedGroupsResponse{}, ReadBodyAsError(res)
	}
	var resp PaginatedGroupsResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

type GroupRequest struct {
	ExcludeMembers bool `json:"exclude_members"`
}

func (p GroupRequest) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if p.ExcludeMembers {
			q.Set("exclude_members", "true")
		}
		r.URL.RawQuery = q.Encode()
	}
}

func (c *Client) Group(ctx context.Context, group uuid.UUID, req GroupRequest) (Group, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups/%s", group.String()),
		nil,
		req.asRequestOption(),
	)
	if err != nil {
		return Group{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Group{}, ReadBodyAsError(res)
	}
	var resp Group
	return resp, ReadBodyAsJSON(res, &resp)
}

func (c *Client) GroupMembers(ctx context.Context, group uuid.UUID, req UsersRequest) (GroupMembersResponse, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups/%s/members", group.String()),
		nil,
		req.Pagination.asRequestOption(),
		req.asRequestOption(),
	)
	if err != nil {
		return GroupMembersResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupMembersResponse{}, ReadBodyAsError(res)
	}
	var resp GroupMembersResponse
	return resp, ReadBodyAsJSON(res, &resp)
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
	return resp, ReadBodyAsJSON(res, &resp)
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
