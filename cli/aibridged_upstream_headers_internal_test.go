//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestProtoToProviderSpecUpstreamHeaders covers the proto-to-runtime half of
// the custom-headers path without a database: the mapping is pure, so a
// dockertest postgres adds nothing.
func TestProtoToProviderSpecUpstreamHeaders(t *testing.T) {
	t.Parallel()

	t.Run("HeadersMapped", func(t *testing.T) {
		t.Parallel()
		spec := protoToProviderSpec(&proto.AIProvider{
			Name:            "zen",
			Type:            string(database.AIProviderTypeOpenaiCompat),
			Enabled:         true,
			BaseUrl:         "https://opencode.ai/zen/go/v1",
			Keys:            []string{"sk-zen"},
			UpstreamHeaders: map[string]string{"x-opencode-session": "{{chat_id}}"},
		})
		require.Equal(t, map[string]string{"x-opencode-session": "{{chat_id}}"}, spec.UpstreamHeaders)
	})

	t.Run("AbsentMeansNil", func(t *testing.T) {
		t.Parallel()
		spec := protoToProviderSpec(&proto.AIProvider{
			Name:    "plain",
			Type:    string(database.AIProviderTypeOpenaiCompat),
			Enabled: true,
			BaseUrl: "https://example.com/v1",
			Keys:    []string{"sk-plain"},
		})
		require.Empty(t, spec.UpstreamHeaders)
	})
}

// TestBuildProviderUpstreamHeaders covers the spec-to-runtime half: an
// openai-compat spec carrying headers builds a provider that exposes them
// for the intercepted and passthrough request paths.
func TestBuildProviderUpstreamHeaders(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	prov, err := buildProvider(ctx, aiProviderSpec{
		Type:            database.AIProviderTypeOpenaiCompat,
		Name:            "zen",
		Enabled:         true,
		BaseURL:         "https://opencode.ai/zen/go/v1",
		Keys:            []string{"sk-zen"},
		UpstreamHeaders: map[string]string{"x-opencode-session": "{{chat_id}}"},
	}, codersdk.AIBridgeConfig{}, nil)
	require.NoError(t, err)

	headers, ok := prov.(interface{ UpstreamHeaders() map[string]string })
	require.True(t, ok, "openai-compat provider must expose custom upstream headers")
	require.Equal(t, map[string]string{"x-opencode-session": "{{chat_id}}"}, headers.UpstreamHeaders())
}
