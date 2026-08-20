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
		"..", "../../../coderd", "../../../coderd/workspaceconnwatcher")
	require.NoError(t, err, "can't parse swagger comments")
	require.NotEmpty(t, swaggerComments, "swagger comments must be present")

	// Coder Tasks has no swagger annotations because it is withdrawn from the
	// product, so verify against a deployment where its routes are not
	// registered.
	values := coderdtest.DeploymentValues(t)
	values.EnableAITasks = false

	//nolint: dogsled
	_, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: values},
	})
	coderdtest.VerifySwaggerDefinitions(t, api.AGPL.APIHandler, swaggerComments, coderdtest.WithSwaggerRoutePrefix("/api/v2"))
}
