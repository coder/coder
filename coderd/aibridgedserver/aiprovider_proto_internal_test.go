package aibridgedserver

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func mustEncodeAIProviderSettings(t *testing.T, s codersdk.AIProviderSettings) sql.NullString {
	t.Helper()
	raw, err := json.Marshal(s)
	require.NoError(t, err)
	if string(raw) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(raw), Valid: true}
}

// TestAIProviderToProtoUpstreamHeaders covers the database-to-proto half of
// the custom-headers path. aiProviderToProto is pure (settings JSON decode),
// so no database is needed.
func TestAIProviderToProtoUpstreamHeaders(t *testing.T) {
	t.Parallel()

	t.Run("EnabledCarriesHeaders", func(t *testing.T) {
		t.Parallel()
		p, err := aiProviderToProto(database.AIProvider{
			Name:    "zen",
			Type:    database.AIProviderTypeOpenaiCompat,
			Enabled: true,
			BaseUrl: "https://opencode.ai/zen/go/v1",
			Settings: mustEncodeAIProviderSettings(t, codersdk.AIProviderSettings{
				UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
					Headers: map[string]string{"x-opencode-session": "{{chat_id}}"},
				},
			}),
		}, nil)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"x-opencode-session": "{{chat_id}}"}, p.GetUpstreamHeaders())
	})

	t.Run("AbsentMeansEmpty", func(t *testing.T) {
		t.Parallel()
		p, err := aiProviderToProto(database.AIProvider{
			Name:    "plain",
			Type:    database.AIProviderTypeOpenaiCompat,
			Enabled: true,
			BaseUrl: "https://example.com/v1",
		}, nil)
		require.NoError(t, err)
		require.Empty(t, p.GetUpstreamHeaders())
	})

	t.Run("DisabledWithholdsHeaders", func(t *testing.T) {
		t.Parallel()
		// Disabled providers never call upstream, so settings stay
		// server-side, matching keys and Bedrock credentials.
		p, err := aiProviderToProto(database.AIProvider{
			Name:    "zen",
			Type:    database.AIProviderTypeOpenaiCompat,
			Enabled: false,
			BaseUrl: "https://opencode.ai/zen/go/v1",
			Settings: mustEncodeAIProviderSettings(t, codersdk.AIProviderSettings{
				UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
					Headers: map[string]string{"x-opencode-session": "{{chat_id}}"},
				},
			}),
		}, nil)
		require.NoError(t, err)
		require.Empty(t, p.GetUpstreamHeaders())
	})
}
