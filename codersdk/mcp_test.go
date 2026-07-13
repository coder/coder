package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestMCPServerConfigOwnerIDJSON(t *testing.T) {
	t.Parallel()

	t.Run("GlobalOmitted", func(t *testing.T) {
		t.Parallel()

		data, err := json.Marshal(codersdk.MCPServerConfig{})
		require.NoError(t, err)
		require.NotContains(t, string(data), "owner_id")
	})

	t.Run("PersonalIncluded", func(t *testing.T) {
		t.Parallel()

		ownerID := uuid.New()
		data, err := json.Marshal(codersdk.MCPServerConfig{OwnerID: ownerID})
		require.NoError(t, err)

		var fields map[string]any
		require.NoError(t, json.Unmarshal(data, &fields))
		require.Equal(t, ownerID.String(), fields["owner_id"])
	})
}
