package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestListPublicLowLevelScopes(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	res, err := client.Request(t.Context(), http.MethodGet, "/api/v2/auth/scopes", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got struct {
		External []string `json:"external"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))

	want := rbac.ExternalScopeNames()
	require.Equal(t, want, got.External)
}
