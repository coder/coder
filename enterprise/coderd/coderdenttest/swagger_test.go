package coderdenttest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
)

func TestEnterpriseEndpointsDocumented(t *testing.T) {
	t.Parallel()

	swaggerComments, err := coderdtest.ParseSwaggerComments("..", "../../../coderd", "../../../coderd/insightsapi")
	require.NoError(t, err, "can't parse swagger comments")
	require.NotEmpty(t, swaggerComments, "swagger comments must be present")

	//nolint: dogsled
	_, _, api, _ := coderdenttest.NewWithAPI(t, nil)
	coderdtest.VerifySwaggerDefinitions(t, api.AGPL.APIHandler, swaggerComments)
}
