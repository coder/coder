package coderdenttest_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
)

func TestNew(t *testing.T) {
	t.Parallel()
	_ = coderdenttest.New(t, nil)
}

func TestAuthorizeAllEndpoints(t *testing.T) {
	if strings.Contains(os.Getenv("CODER_EXPERIMENTS_TEST"), string(codersdk.ExperimentAuthzQuerier)) {
		t.Skip("Skipping TestAuthorizeAllEndpoints for authz_querier experiment")
	}
	t.Parallel()
	client, _, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			// Required for any subdomain-based proxy tests to pass.
			AppHostname:              "*.test.coder.com",
			Authorizer:               &coderdtest.RecordingAuthorizer{Wrapped: &coderdtest.FakeAuthorizer{}},
			IncludeProvisionerDaemon: true,
		},
	})
	ctx, _ := testutil.Context(t)
	admin := coderdtest.CreateFirstUser(t, client)
	lic := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureTemplateRBAC:               1,
			codersdk.FeatureExternalProvisionerDaemons: 1,
		},
	})
	group, err := client.CreateGroup(ctx, admin.OrganizationID, codersdk.CreateGroupRequest{
		Name: "testgroup",
	})
	require.NoError(t, err)

	groupObj := rbac.ResourceGroup.WithID(group.ID).InOrg(admin.OrganizationID)
	a := coderdtest.NewAuthTester(ctx, t, client, api.AGPL, admin)
	a.URLParams["licenses/{id}"] = fmt.Sprintf("licenses/%d", lic.ID)
	a.URLParams["groups/{group}"] = fmt.Sprintf("groups/%s", group.ID.String())
	a.URLParams["{groupName}"] = group.Name

	skipRoutes, assertRoute := coderdtest.AGPLRoutes(a)
	skipRoutes["GET:/api/v2/organizations/{organization}/provisionerdaemons/serve"] = "This route checks for RBAC dependent on input parameters!"
	skipRoutes["GET:/api/v2/appearance/"] = "This route is available to all users"

	assertRoute["GET:/api/v2/entitlements"] = coderdtest.RouteCheck{
		NoAuthorize: true,
	}
	assertRoute["POST:/api/v2/licenses"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionCreate,
		AssertObject: rbac.ResourceLicense,
	}
	assertRoute["GET:/api/v2/licenses"] = coderdtest.RouteCheck{
		StatusCode:   http.StatusOK,
		AssertAction: rbac.ActionRead,
		AssertObject: rbac.ResourceLicense,
	}
	assertRoute["GET:/api/v2/replicas"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionRead,
		AssertObject: rbac.ResourceReplicas,
	}
	assertRoute["DELETE:/api/v2/licenses/{id}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionDelete,
		AssertObject: rbac.ResourceLicense,
	}
	assertRoute["GET:/api/v2/templates/{template}/acl"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionRead,
		AssertObject: rbac.ResourceTemplate,
	}
	assertRoute["PATCH:/api/v2/templates/{template}/acl"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionCreate,
		AssertObject: rbac.ResourceTemplate,
	}
	assertRoute["GET:/api/v2/organizations/{organization}/groups"] = coderdtest.RouteCheck{
		StatusCode:   http.StatusOK,
		AssertAction: rbac.ActionRead,
		AssertObject: groupObj,
	}
	assertRoute["GET:/api/v2/organizations/{organization}/groups/{groupName}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionRead,
		AssertObject: groupObj,
	}
	assertRoute["GET:/api/v2/organizations/{organization}/provisionerdaemons"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionRead,
		AssertObject: rbac.ResourceProvisionerDaemon,
		StatusCode:   http.StatusOK,
	}
	assertRoute["GET:/api/v2/groups/{group}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionRead,
		AssertObject: groupObj,
	}
	assertRoute["PATCH:/api/v2/groups/{group}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionUpdate,
		AssertObject: groupObj,
	}
	assertRoute["DELETE:/api/v2/groups/{group}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionDelete,
		AssertObject: groupObj,
	}

	a.Test(context.Background(), assertRoute, skipRoutes)
}
