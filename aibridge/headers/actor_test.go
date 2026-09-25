package headers_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestActorHeaders(t *testing.T) {
	t.Parallel()

	require.Equal(t, "X-AI-Bridge-Actor-ID", headers.ActorIDHeader())
	require.Equal(t, "X-AI-Bridge-Actor-Metadata-Username", headers.ActorMetadataHeader("Username"))

	require.True(t, headers.IsActorHeader(headers.ActorIDHeader()))
	require.True(t, headers.IsActorHeader("x-ai-bridge-actor-metadata-name"))
	require.False(t, headers.IsActorHeader("X-AI-Bridge-Request-ID"))
}

func TestFromActor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		actor *aibcontext.Actor
		want  map[string]string
	}{
		{name: "nil actor"},
		{
			name:  "id only",
			actor: &aibcontext.Actor{ID: "user-123"},
			want:  map[string]string{"X-AI-Bridge-Actor-ID": "user-123"},
		},
		{
			name: "metadata",
			actor: &aibcontext.Actor{
				ID: "user-123",
				Metadata: recorder.Metadata{
					"Username": "alice",
					"Count":    42,
				},
			},
			want: map[string]string{
				"X-AI-Bridge-Actor-ID":                "user-123",
				"X-AI-Bridge-Actor-Metadata-Username": "alice",
				"X-AI-Bridge-Actor-Metadata-Count":    "42",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, headers.FromActor(tc.actor))
		})
	}
}
