package coderdenttest_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestNew(t *testing.T) {
	t.Parallel()
	_ = coderdenttest.New(t, nil)
}

func TestAuthorizeAllEndpoints(t *testing.T) {
	t.Parallel()
	client, _, api := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			// Required for any subdomain-based proxy tests to pass.
			AppHostname:              "test.coder.com",
			Authorizer:               &coderdtest.RecordingAuthorizer{},
			IncludeProvisionerDaemon: true,
		},
	})
	admin := coderdtest.CreateFirstUser(t, client)
	license := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{})
	a := coderdtest.NewAuthTester(context.Background(), t, client, api.AGPL, admin)
	a.URLParams["licenses/{id}"] = fmt.Sprintf("licenses/%d", license.ID)

	skipRoutes, assertRoute := coderdtest.AGPLRoutes(a)
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
	assertRoute["DELETE:/api/v2/licenses/{id}"] = coderdtest.RouteCheck{
		AssertAction: rbac.ActionDelete,
		AssertObject: rbac.ResourceLicense,
	}

	a.Test(context.Background(), assertRoute, skipRoutes)
}
