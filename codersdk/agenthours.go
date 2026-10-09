package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AgentHoursAllotmentMaxBps is the largest Agent Hours allotment, in basis
// points, and the cap on the sum of allotments within one tier. 10000 basis
// points equal 100%.
const AgentHoursAllotmentMaxBps = 10000

// AgentHoursOrganizationAllotment is an organization's share of the
// deployment's licensed Agent Hours. Allotments are configuration only and
// are not enforced.
type AgentHoursOrganizationAllotment struct {
	OrganizationID          uuid.UUID `json:"organization_id" format:"uuid"`
	OrganizationName        string    `json:"organization_name"`
	OrganizationDisplayName string    `json:"organization_display_name"`
	// AllotmentBps is the share in basis points (10000 = 100%).
	AllotmentBps int32     `json:"allotment_bps" example:"2500"`
	CreatedAt    time.Time `json:"created_at" format:"date-time"`
	UpdatedAt    time.Time `json:"updated_at" format:"date-time"`
}

// AgentHoursGroupAllotment is a group's share of its organization's Agent
// Hours. Allotments are configuration only and are not enforced.
type AgentHoursGroupAllotment struct {
	GroupID          uuid.UUID `json:"group_id" format:"uuid"`
	GroupName        string    `json:"group_name"`
	GroupDisplayName string    `json:"group_display_name"`
	// AllotmentBps is the share in basis points (10000 = 100%).
	AllotmentBps int32     `json:"allotment_bps" example:"2500"`
	CreatedAt    time.Time `json:"created_at" format:"date-time"`
	UpdatedAt    time.Time `json:"updated_at" format:"date-time"`
}

// AgentHoursGroupAllotments lists the group allotments of one organization.
type AgentHoursGroupAllotments struct {
	// OrganizationAllotmentBps is the organization's own share of the
	// deployment's Agent Hours. It is null when the organization has no
	// allotment and draws from the shared pool.
	OrganizationAllotmentBps *int32                     `json:"organization_allotment_bps" example:"2500"`
	Groups                   []AgentHoursGroupAllotment `json:"groups"`
}

// UpsertAgentHoursAllotmentRequest sets an Agent Hours allotment.
type UpsertAgentHoursAllotmentRequest struct {
	// AllotmentBps is the share in basis points, from 1 to
	// AgentHoursAllotmentMaxBps.
	AllotmentBps int32 `json:"allotment_bps" example:"2500"`
}

// AgentHoursOrganizationAllotments lists every organization's Agent Hours
// allotment.
func (c *Client) AgentHoursOrganizationAllotments(ctx context.Context) ([]AgentHoursOrganizationAllotment, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/agent-hours/allotments", nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []AgentHoursOrganizationAllotment
	return resp, ReadBodyAsJSON(res, &resp)
}

// UpsertAgentHoursOrganizationAllotment sets an organization's share of the
// deployment's Agent Hours.
func (c *Client) UpsertAgentHoursOrganizationAllotment(ctx context.Context, organizationID uuid.UUID, req UpsertAgentHoursAllotmentRequest) (AgentHoursOrganizationAllotment, error) {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/organizations/%s/agent-hours/allotment", organizationID.String()),
		req,
	)
	if err != nil {
		return AgentHoursOrganizationAllotment{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentHoursOrganizationAllotment{}, ReadBodyAsError(res)
	}
	var resp AgentHoursOrganizationAllotment
	return resp, ReadBodyAsJSON(res, &resp)
}

// DeleteAgentHoursOrganizationAllotment removes an organization's Agent
// Hours allotment.
func (c *Client) DeleteAgentHoursOrganizationAllotment(ctx context.Context, organizationID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/organizations/%s/agent-hours/allotment", organizationID.String()),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// AgentHoursGroupAllotments lists the Agent Hours allotments of the groups in
// an organization.
func (c *Client) AgentHoursGroupAllotments(ctx context.Context, organizationID uuid.UUID) (AgentHoursGroupAllotments, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/agent-hours/group-allotments", organizationID.String()),
		nil,
	)
	if err != nil {
		return AgentHoursGroupAllotments{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentHoursGroupAllotments{}, ReadBodyAsError(res)
	}
	var resp AgentHoursGroupAllotments
	return resp, ReadBodyAsJSON(res, &resp)
}

// UpsertAgentHoursGroupAllotment sets a group's share of its organization's
// Agent Hours.
func (c *Client) UpsertAgentHoursGroupAllotment(ctx context.Context, groupID uuid.UUID, req UpsertAgentHoursAllotmentRequest) (AgentHoursGroupAllotment, error) {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/groups/%s/agent-hours/allotment", groupID.String()),
		req,
	)
	if err != nil {
		return AgentHoursGroupAllotment{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AgentHoursGroupAllotment{}, ReadBodyAsError(res)
	}
	var resp AgentHoursGroupAllotment
	return resp, ReadBodyAsJSON(res, &resp)
}

// DeleteAgentHoursGroupAllotment removes a group's Agent Hours allotment.
func (c *Client) DeleteAgentHoursGroupAllotment(ctx context.Context, groupID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/groups/%s/agent-hours/allotment", groupID.String()),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
