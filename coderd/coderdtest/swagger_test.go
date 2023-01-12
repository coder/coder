package coderdtest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestEndpointsDocumented(t *testing.T) {
	t.Parallel()

	swaggerComments, err := coderdtest.ParseSwaggerComments("..")
	require.NoError(t, err, "can't parse swagger comments")

	_, _, api := coderdtest.NewWithAPI(t, nil)
	coderdtest.VerifySwaggerDefinitions(t, api.APIHandler, swaggerComments)
}
