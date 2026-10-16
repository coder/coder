package headers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/context"
)

func TestHeadersFromActor(t *testing.T) {
	t.Parallel()

	actorID := uuid.MustParse("6f1d2c3b-4a5e-4f60-8a7b-9c0d1e2f3a4b")
	const apiKeyID = "api-key-id"
	for _, tc := range []struct {
		name  string
		actor *context.Actor
		names map[string]string
		want  map[string]string
	}{
		{name: "nil actor", names: map[string]string{"id": ActorIDHeader}},
		{
			name:  "id only",
			actor: &context.Actor{ID: actorID},
			names: map[string]string{"id": "X-Downstream-User-Id"},
			want:  map[string]string{"X-Downstream-User-Id": actorID.String()},
		},
		{
			name: "configured supported attributes only",
			actor: &context.Actor{
				ID:       actorID,
				APIKeyID: apiKeyID,
				Username: "alice",
				Email:    "alice@example.com",
			},
			names: map[string]string{
				"id":         "X-Downstream-User-Id",
				"username":   "X-Downstream-Username",
				"email":      "X-Downstream-Email",
				"Role":       "X-Downstream-Role",
				"api_key_id": "X-Downstream-Api-Key-Id",
				"APIKeyID":   "X-Downstream-Api-Key",
			},
			want: map[string]string{
				"X-Downstream-User-Id":  actorID.String(),
				"X-Downstream-Username": "alice",
				"X-Downstream-Email":    "alice@example.com",
			},
		},
		{
			name:  "id omitted",
			actor: &context.Actor{ID: actorID, Email: "alice@example.com", Username: "alice"},
			names: map[string]string{"username": "X-Downstream-Username", "email": "X-Downstream-Email"},
			want:  map[string]string{"X-Downstream-Username": "alice", "X-Downstream-Email": "alice@example.com"},
		},
		{
			name:  "username omitted",
			actor: &context.Actor{ID: actorID, Email: "alice@example.com", Username: "alice"},
			names: map[string]string{"id": "X-Downstream-User-Id", "email": "X-Downstream-Email"},
			want:  map[string]string{"X-Downstream-User-Id": actorID.String(), "X-Downstream-Email": "alice@example.com"},
		},
		{
			name:  "email omitted",
			actor: &context.Actor{ID: actorID, Email: "alice@example.com", Username: "alice"},
			names: map[string]string{"id": "X-Downstream-User-Id", "username": "X-Downstream-Username"},
			want:  map[string]string{"X-Downstream-User-Id": actorID.String(), "X-Downstream-Username": "alice"},
		},
		{
			name:  "missing username",
			actor: &context.Actor{ID: actorID},
			names: map[string]string{"username": "X-Username"},
			want:  map[string]string{},
		},
		{
			name:  "nil id",
			actor: &context.Actor{Username: "alice"},
			names: map[string]string{"id": "X-User-Id"},
			want:  map[string]string{},
		},
		{
			name:  "empty email",
			actor: &context.Actor{ID: actorID},
			names: map[string]string{"email": "X-Email"},
			want:  map[string]string{},
		},
		{
			name:  "unknown attribute keys forward nothing",
			actor: &context.Actor{ID: actorID, APIKeyID: apiKeyID, Username: "alice", Email: "alice@example.com"},
			names: map[string]string{"Username": "X-Username", "ID": "X-Id", "api_key_id": "X-Api-Key-Id"},
			want:  map[string]string{},
		},
		{
			name:  "empty map forwards no actor headers",
			actor: &context.Actor{ID: actorID, Username: "alice"},
			names: map[string]string{},
			want:  map[string]string{},
		},
		{
			name:  "nil map forwards no actor headers",
			actor: &context.Actor{ID: actorID},
			want:  map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := headersFromActor(tc.actor, tc.names)
			require.Equal(t, tc.want, got)
			for name, value := range got {
				require.NotEqual(t, apiKeyID, value, "the API key ID must never be forwarded in %s", name)
			}
		})
	}
}
