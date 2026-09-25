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
		names map[string]string
		want  map[string]string
	}{
		{name: "nil actor", names: map[string]string{"id": ActorIDHeader()}},
		{
			name:  "id only",
			actor: &context.Actor{ID: "user-123"},
			names: map[string]string{"id": "X-Downstream-User-Id"},
			want:  map[string]string{"X-Downstream-User-Id": "user-123"},
		},
		{
			name: "configured supported attributes only",
			actor: &context.Actor{
				ID: "user-123",
				Metadata: recorder.Metadata{
					"Username": "alice",
					"Count":    42,
				},
			},
			names: map[string]string{
				"id":       "X-Downstream-User-Id",
				"username": "X-Downstream-Username",
			},
			want: map[string]string{
				"X-Downstream-User-Id":  "user-123",
				"X-Downstream-Username": "alice",
			},
		},
		{
			name:  "id omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}},
			names: map[string]string{"username": "X-Downstream-Username"},
			want:  map[string]string{"X-Downstream-Username": "alice"},
		},
		{
			name:  "username omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}},
			names: map[string]string{"id": "X-Downstream-User-Id"},
			want:  map[string]string{"X-Downstream-User-Id": "user-123"},
		},
		{
			name:  "missing username",
			actor: &context.Actor{ID: "user-123"},
			names: map[string]string{"username": "X-Username"},
			want:  map[string]string{},
		},
		{
			name:  "empty username",
			actor: &context.Actor{Metadata: recorder.Metadata{"Username": ""}},
			names: map[string]string{"username": "X-Username"},
			want:  map[string]string{},
		},
		{
			name:  "non-string username",
			actor: &context.Actor{Metadata: recorder.Metadata{"Username": 42}},
			names: map[string]string{"username": "X-Username"},
			want:  map[string]string{},
		},
		{
			name: "empty map forwards no actor headers",
			actor: &context.Actor{
				ID: "user-123",
				Metadata: recorder.Metadata{
					"Username": "alice",
				},
			},
			names: map[string]string{},
			want:  map[string]string{},
		},
		{
			name:  "nil map forwards no actor headers",
			actor: &context.Actor{ID: "user-123"},
			want:  map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, headersFromActor(tc.actor, tc.names))
		})
	}
}
