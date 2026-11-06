package coderdenttest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
)

func TestEnterpriseEndpointsDocumented(t *testing.T) {
	t.Parallel()

	swaggerComments, err := coderdtest.ParseSwaggerComments(
		"..", "../legacyscim", "../../../coderd", "../../../coderd/workspaceconnwatcher", "../../../coderd/workspaceapps")
	require.NoError(t, err, "can't parse swagger comments")
	require.NotEmpty(t, swaggerComments, "swagger comments must be present")

	//nolint: dogsled
	_, _, api, _ := coderdenttest.NewWithAPI(t, nil)
	coderdtest.VerifySwaggerDefinitions(t, api.AGPL.APIHandler, swaggerComments,
		coderdtest.WithSwaggerRoutePrefix("/api/v2"),
		coderdtest.WithSwaggerDescriptionExemptions(enterpriseRoutesWithoutDescription))
}

// enterpriseRoutesWithoutDescription lists enterprise routes, formatted as
// "<method> <route>", that the API reference publishes without a
// @Description. They predate the requirement in
// coderdtest.VerifySwaggerDefinitions. Do not add routes here: give a new
// route a @Description, and remove a route from this list when you add one.
var enterpriseRoutesWithoutDescription = []string{
	"get /api/v2/agent-firewall/sessions/{id}",
	"get /api/v2/agent-firewall/sessions/{id}/logs",
	"get /api/v2/ai-gateway/keys",
	"post /api/v2/ai-gateway/keys",
	"delete /api/v2/ai-gateway/keys/{key}",
	"get /api/v2/ai-gateway/providers",
	"get /api/v2/ai-gateway/serve",
	"get /api/v2/appearance",
	"put /api/v2/appearance",
	"get /api/v2/connectionlog",
	"get /api/v2/entitlements",
	"get /api/v2/groups",
	"delete /api/v2/groups/{group}",
	"get /api/v2/groups/{group}",
	"patch /api/v2/groups/{group}",
	"delete /api/v2/groups/{group}/ai/budget",
	"get /api/v2/groups/{group}/ai/budget",
	"put /api/v2/groups/{group}/ai/budget",
	"get /api/v2/groups/{group}/members",
	"get /api/v2/licenses",
	"post /api/v2/licenses",
	"post /api/v2/licenses/refresh-entitlements",
	"post /api/v2/licenses/trial",
	"delete /api/v2/licenses/{id}",
	"put /api/v2/notifications/templates/{notification_template}/method",
	"post /api/v2/organizations",
	"delete /api/v2/organizations/{organization}",
	"patch /api/v2/organizations/{organization}",
	"get /api/v2/organizations/{organization}/groups",
	"post /api/v2/organizations/{organization}/groups",
	"get /api/v2/organizations/{organization}/groups/{groupName}",
	"get /api/v2/organizations/{organization}/groups/{groupName}/members",
	"post /api/v2/organizations/{organization}/members/roles",
	"put /api/v2/organizations/{organization}/members/roles",
	"delete /api/v2/organizations/{organization}/members/roles/{roleName}",
	"get /api/v2/organizations/{organization}/members/{user}/workspace-quota",
	"get /api/v2/organizations/{organization}/provisionerdaemons/serve",
	"get /api/v2/organizations/{organization}/provisionerkeys",
	"post /api/v2/organizations/{organization}/provisionerkeys",
	"get /api/v2/organizations/{organization}/provisionerkeys/daemons",
	"delete /api/v2/organizations/{organization}/provisionerkeys/{provisionerkey}",
	"get /api/v2/organizations/{organization}/settings/idpsync/available-fields",
	"get /api/v2/organizations/{organization}/settings/idpsync/field-values",
	"get /api/v2/organizations/{organization}/settings/idpsync/groups",
	"patch /api/v2/organizations/{organization}/settings/idpsync/groups",
	"patch /api/v2/organizations/{organization}/settings/idpsync/groups/config",
	"patch /api/v2/organizations/{organization}/settings/idpsync/groups/mapping",
	"get /api/v2/organizations/{organization}/settings/idpsync/roles",
	"patch /api/v2/organizations/{organization}/settings/idpsync/roles",
	"patch /api/v2/organizations/{organization}/settings/idpsync/roles/config",
	"patch /api/v2/organizations/{organization}/settings/idpsync/roles/mapping",
	"get /api/v2/organizations/{organization}/settings/workspace-sharing",
	"patch /api/v2/organizations/{organization}/settings/workspace-sharing",
	"get /api/v2/prebuilds/settings",
	"put /api/v2/prebuilds/settings",
	"get /api/v2/provisionerkeys/{provisionerkey}",
	"get /api/v2/replicas",
	"get /api/v2/settings/idpsync/available-fields",
	"get /api/v2/settings/idpsync/field-values",
	"get /api/v2/settings/idpsync/organization",
	"patch /api/v2/settings/idpsync/organization",
	"patch /api/v2/settings/idpsync/organization/config",
	"patch /api/v2/settings/idpsync/organization/mapping",
	"get /api/v2/templates/{template}/acl",
	"patch /api/v2/templates/{template}/acl",
	"get /api/v2/templates/{template}/acl/available",
	"post /api/v2/templates/{template}/prebuilds/invalidate",
	"delete /api/v2/users/{user}/ai/budget/override",
	"get /api/v2/users/{user}/ai/budget/override",
	"put /api/v2/users/{user}/ai/budget/override",
	"get /api/v2/users/{user}/ai/spend",
	"get /api/v2/users/{user}/quiet-hours",
	"put /api/v2/users/{user}/quiet-hours",
	"get /api/v2/workspace-quota/{user}",
	"get /api/v2/workspaceproxies",
	"post /api/v2/workspaceproxies",
	"delete /api/v2/workspaceproxies/{workspaceproxy}",
	"get /api/v2/workspaceproxies/{workspaceproxy}",
	"patch /api/v2/workspaceproxies/{workspaceproxy}",
	"get /api/v2/workspaces/{workspace}/external-agent/{agent}/credentials",
	"get /scim/v2/ServiceProviderConfig",
	"get /scim/v2/Users",
	"post /scim/v2/Users",
	"get /scim/v2/Users/{id}",
	"patch /scim/v2/Users/{id}",
	"put /scim/v2/Users/{id}",
}
