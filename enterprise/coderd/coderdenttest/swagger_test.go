package coderdenttest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"fmt"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
)

func TestEnterpriseEndpointsDocumented(t *testing.T) {
	t.Parallel()

	fmt.Println("wtf")
	swaggerComments, err := coderdtest.ParseSwaggerComments("..", "../../../coderd")
	require.NoError(t, err, "can't parse swagger comments")
	require.NotEmpty(t, swaggerComments, "swagger comments must be present")
	fmt.Println("wtf2")

	//nolint: dogsled
	_, _, api, _ := coderdenttest.NewWithAPI(t, nil)
	fmt.Println("wtf3")
	coderdtest.VerifySwaggerDefinitions(t, api.AGPL.APIHandler, swaggerComments)
	fmt.Println("wtf4")
}
