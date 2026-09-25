package headers_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/headers"
)

func TestActorHeaders(t *testing.T) {
	t.Parallel()

	require.Equal(t, "X-AI-Bridge-Actor-ID", headers.ActorIDHeader())
	require.Equal(t, "X-AI-Bridge-Actor-Metadata-Username", headers.ActorMetadataHeader("Username"))

	require.True(t, headers.IsActorHeader(headers.ActorIDHeader()))
	require.True(t, headers.IsActorHeader("x-ai-bridge-actor-metadata-name"))
	require.False(t, headers.IsActorHeader("X-AI-Bridge-Request-ID"))
}
