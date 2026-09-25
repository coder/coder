package intercept

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestHeadersFromActor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		actor *context.Actor
		want  map[string]string
	}{
		{name: "nil actor"},
		{
			name:  "id only",
			actor: &context.Actor{ID: "user-123"},
			want:  map[string]string{"X-AI-Bridge-Actor-ID": "user-123"},
		},
		{
			name: "metadata",
			actor: &context.Actor{
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
			require.Equal(t, tc.want, headersFromActor(tc.actor))
		})
	}
}
