package coderd_test

import (
	"context"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/stretchr/testify/require"
)

func TestLoginTypes(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	loginTypes, err := client.LoginTypes(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, len(loginTypes), 1)
	require.EqualValues(t, loginTypes[0], codersdk.LoginType{
		Type: "built-in",
	})
}
