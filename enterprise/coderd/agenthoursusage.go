package coderd

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// maxGroupMembersAgentHoursUserIDs caps the user IDs of one group members
// Agent Hours request.
const maxGroupMembersAgentHoursUserIDs = 100

// agentHoursUsagePeriod returns the license usage period that Agent Hours
// usage is reported for. A bucket counts in the period that contains its
// start, as it does for the license total.
func (api *API) agentHoursUsagePeriod(ctx context.Context, rw http.ResponseWriter) (codersdk.UsagePeriod, bool) {
	feature, ok := api.Entitlements.Feature(codersdk.FeatureAgentRuntimeHours)
	if !ok || feature.UsagePeriod == nil {
		// Licenses that enable Agent Hours always carry a usage period.
		api.Logger.Error(ctx, "agent hours entitlement has no usage period")
		httpapi.InternalServerError(rw, xerrors.New("Agent Hours has no license usage period"))
		return codersdk.UsagePeriod{}, false
	}
	return *feature.UsagePeriod, true
}

// @Summary Get Agent Hours usage
// @ID get-agent-hours-usage
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.AgentHoursUsage
// @Router /api/v2/agent-hours/usage [get]
func (api *API) agentHoursUsage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	period, ok := api.agentHoursUsagePeriod(ctx, rw)
	if !ok {
		return
	}

	rows, err := api.Database.GetAgentRuntimeUsageByOrganization(ctx, database.GetAgentRuntimeUsageByOrganizationParams{
		StartTime: period.Start,
		EndTime:   period.End,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	//nolint:gocritic // The caller can read the deployment's usage; reading usage events requires the usage publisher subject.
	totalMs, err := api.Database.GetTotalUsageHBAgentRuntimeV1(dbauthz.AsUsagePublisher(ctx), database.GetTotalUsageHBAgentRuntimeV1Params{
		StartTime: period.Start,
		EndTime:   period.End,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.AgentHoursUsage{
		UsagePeriod:   period,
		TotalMs:       totalMs,
		Organizations: make([]codersdk.AgentHoursOrganizationUsage, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Organizations = append(resp.Organizations, codersdk.AgentHoursOrganizationUsage{
			OrganizationID:          row.OrganizationID,
			OrganizationName:        row.OrganizationName,
			OrganizationDisplayName: row.OrganizationDisplayName,
			UsedMs:                  row.RuntimeMs,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get organization Agent Hours usage
// @ID get-organization-agent-hours-usage
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.AgentHoursOrganizationGroupsUsage
// @Router /api/v2/organizations/{organization}/agent-hours/usage [get]
func (api *API) organizationAgentHoursUsage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceGroup.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}
	if org.Deleted {
		httpapi.ResourceNotFound(rw)
		return
	}
	period, ok := api.agentHoursUsagePeriod(ctx, rw)
	if !ok {
		return
	}

	rows, err := api.Database.GetAgentRuntimeUsageByGroup(ctx, database.GetAgentRuntimeUsageByGroupParams{
		OrganizationID: org.ID,
		StartTime:      period.Start,
		EndTime:        period.End,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.AgentHoursOrganizationGroupsUsage{
		UsagePeriod: period,
		Groups:      make([]codersdk.AgentHoursGroupUsage, 0, len(rows)),
	}
	for _, row := range rows {
		resp.UsedMs += row.RuntimeMs
		resp.Groups = append(resp.Groups, codersdk.AgentHoursGroupUsage{
			GroupID:          row.GroupID,
			GroupName:        row.GroupName,
			GroupDisplayName: row.GroupDisplayName,
			UsedMs:           row.RuntimeMs,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary Get group members Agent Hours usage
// @Description Returns, per requested user, the Agent Hours that counted toward the group in the license usage period and the group the user's Agent Hours count toward now.
// @Description A maximum of 100 user IDs may be requested per call, and requests with more are rejected, so callers are expected to batch across multiple requests.
// @Description User IDs that are not members of the group, or that the caller has no read access to, are silently omitted.
// @ID get-group-members-agent-hours-usage
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Param user_ids query string true "Comma-separated list of user IDs (maximum 100)"
// @Success 200 {object} codersdk.AgentHoursGroupMembersUsage
// @Router /api/v2/groups/{group}/members/agent-hours [get]
func (api *API) groupMembersAgentHoursUsage(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := httpmw.GroupParam(r)

	parser := httpapi.NewQueryParamParser()
	parser.RequiredNotEmpty("user_ids")
	userIDs := parser.UUIDs(r.URL.Query(), nil, "user_ids")
	parser.ErrorExcessParams(r.URL.Query())
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}
	if len(userIDs) > maxGroupMembersAgentHoursUserIDs {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("user_ids has %d entries, maximum is %d.", len(userIDs), maxGroupMembersAgentHoursUserIDs),
		})
		return
	}
	period, ok := api.agentHoursUsagePeriod(ctx, rw)
	if !ok {
		return
	}

	rows, err := api.Database.GetGroupMembersAgentRuntimeUsage(ctx, database.GetGroupMembersAgentRuntimeUsageParams{
		GroupID:   group.ID,
		UserIds:   userIDs,
		StartTime: period.Start,
		EndTime:   period.End,
	})
	if err != nil {
		api.Logger.Error(ctx, "get group members agent hours usage", slog.F("group_id", group.ID), slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := codersdk.AgentHoursGroupMembersUsage{
		UsagePeriod: period,
		Members:     make([]codersdk.AgentHoursGroupMemberUsage, 0, len(rows)),
	}
	for _, row := range rows {
		resp.Members = append(resp.Members, codersdk.AgentHoursGroupMemberUsage{
			UserID: row.UserID,
			UsedMs: row.RuntimeMs,
			EffectiveGroup: codersdk.AgentHoursEffectiveGroup{
				ID:          row.EffectiveGroupID,
				Name:        row.EffectiveGroupName,
				DisplayName: row.EffectiveGroupDisplayName,
			},
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
