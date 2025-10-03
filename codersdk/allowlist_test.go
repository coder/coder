package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func TestAPIAllowListTarget_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	all := codersdk.AllowAllTarget()
	b, err := json.Marshal(all)
	require.NoError(t, err)
	require.JSONEq(t, `"*:*"`, string(b))
	var rt codersdk.APIAllowListTarget
	require.NoError(t, json.Unmarshal(b, &rt))
	require.Equal(t, codersdk.ResourceWildcard, rt.Type)
	require.Equal(t, policy.WildcardSymbol, rt.ID)

	ty := codersdk.AllowTypeTarget(codersdk.ResourceWorkspace)
	b, err = json.Marshal(ty)
	require.NoError(t, err)
	require.JSONEq(t, `"workspace:*"`, string(b))
	require.NoError(t, json.Unmarshal(b, &rt))
	require.Equal(t, codersdk.ResourceWorkspace, rt.Type)
	require.Equal(t, policy.WildcardSymbol, rt.ID)

	id := uuid.New()
	res := codersdk.AllowResourceTarget(codersdk.ResourceTemplate, id)
	b, err = json.Marshal(res)
	require.NoError(t, err)
	exp := `"template:` + id.String() + `"`
	require.JSONEq(t, exp, string(b))
}
