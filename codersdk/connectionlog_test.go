package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// An absent app is omitted rather than sent empty.
func TestConnectionLogJSON(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(codersdk.ConnectionLog{ConnectionMethod: codersdk.ConnectionLogMethodSSH})
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"app_name"`)
	require.NotContains(t, string(raw), `"app_display_name"`)
}
