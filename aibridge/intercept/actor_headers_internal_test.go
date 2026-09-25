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
					"Email":    "alice@example.com",
					"Count":    42,
				},
			},
			names: map[string]string{
				"id":       "X-Downstream-User-Id",
				"username": "X-Downstream-Username",
				"email":    "X-Downstream-Email",
			},
			want: map[string]string{
				"X-Downstream-User-Id":  "user-123",
				"X-Downstream-Username": "alice",
				"X-Downstream-Email":    "alice@example.com",
			},
		},
		{
			name:  "id omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice", "Email": "alice@example.com"}},
			names: map[string]string{"username": "X-Downstream-Username", "email": "X-Downstream-Email"},
			want:  map[string]string{"X-Downstream-Username": "alice", "X-Downstream-Email": "alice@example.com"},
		},
		{
			name:  "username omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice", "Email": "alice@example.com"}},
			names: map[string]string{"id": "X-Downstream-User-Id", "email": "X-Downstream-Email"},
			want:  map[string]string{"X-Downstream-User-Id": "user-123", "X-Downstream-Email": "alice@example.com"},
		},
		{
			name:  "email omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice", "Email": "alice@example.com"}},
			names: map[string]string{"id": "X-Downstream-User-Id", "username": "X-Downstream-Username"},
			want:  map[string]string{"X-Downstream-User-Id": "user-123", "X-Downstream-Username": "alice"},
		},
		{
			name:  "empty email omitted",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice", "Email": ""}},
			names: map[string]string{"id": "X-Downstream-User-Id", "email": "X-Downstream-Email"},
			want:  map[string]string{"X-Downstream-User-Id": "user-123"},
		},
		{
			name: "empty map forwards no actor headers",
			actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{
				"Username": "alice",
				"Email":    "alice@example.com",
			}},
			names: map[string]string{},
			want:  map[string]string{},
		},
		{name: "nil map forwards no actor headers", actor: &context.Actor{ID: "user-123"}, want: map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, headersFromActor(tc.actor, tc.names))
		})
	}
}

func TestHeadersFromActorUsesConfiguredNames(t *testing.T) {
	t.Parallel()

	actor := &context.Actor{ID: "user-123", Metadata: recorder.Metadata{
		"Username": "alice",
		"Email":    "alice@example.com",
		"Plan":     "pro",
	}}

	require.Equal(t, map[string]string{
		"X-Downstream-User-Id":  "user-123",
		"X-Downstream-Username": "alice",
		"X-Downstream-Email":    "alice@example.com",
	}, headersFromActor(actor, map[string]string{
		"id":       "X-Downstream-User-Id",
		"username": "X-Downstream-Username",
		"email":    "X-Downstream-Email",
	}))
}
