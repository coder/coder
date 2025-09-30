package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestListPublicLowLevelScopes(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	res, err := client.Request(t.Context(), http.MethodGet, "/api/v2/auth/scopes", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got codersdk.ScopeCatalog
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))

	expectedSpecials := rbac.ExternalSpecialScopes()
	convertedSpecials := make([]codersdk.APIKeyScope, 0, len(expectedSpecials))
	for _, scope := range expectedSpecials {
		convertedSpecials = append(convertedSpecials, codersdk.APIKeyScope(scope))
	}
	require.Equal(t, convertedSpecials, got.Specials)

	lowMeta := rbac.ExternalLowLevelCatalog()
	expectedLow := make([]codersdk.ScopeCatalogLowLevel, 0, len(lowMeta))
	for _, meta := range lowMeta {
		expectedLow = append(expectedLow, codersdk.ScopeCatalogLowLevel{
			Name:     codersdk.APIKeyScope(meta.Name),
			Resource: codersdk.RBACResource(meta.Resource),
			Action:   string(meta.Action),
		})
	}
	require.Equal(t, expectedLow, got.LowLevel)

	compMeta := rbac.ExternalCompositeCatalog()
	expectedComposites := make([]codersdk.ScopeCatalogComposite, 0, len(compMeta))
	for _, meta := range compMeta {
		expands := make([]codersdk.APIKeyScope, 0, len(meta.ExpandsTo))
		for _, name := range meta.ExpandsTo {
			expands = append(expands, codersdk.APIKeyScope(name))
		}
		expectedComposites = append(expectedComposites, codersdk.ScopeCatalogComposite{
			Name:      codersdk.APIKeyScope(meta.Name),
			ExpandsTo: expands,
		})
	}
	require.Equal(t, expectedComposites, got.Composites)
}
