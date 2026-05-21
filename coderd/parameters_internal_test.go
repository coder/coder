package coderd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestNextDynamicParametersResponseID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		lastResponseID int
		requestID      int
		want           int
	}{
		{
			name:           "request ID advances response ID",
			lastResponseID: 1,
			requestID:      4,
			want:           4,
		},
		{
			name:           "request ID collision advances response ID",
			lastResponseID: 4,
			requestID:      4,
			want:           5,
		},
		{
			name:           "stale request ID advances response ID",
			lastResponseID: 4,
			requestID:      2,
			want:           5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := nextDynamicParametersResponseID(tt.lastResponseID, tt.requestID)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCanSubscribeUserSecretEventsRequiresSecretRead(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	actor := rbac.Subject{ID: uuid.NewString()}

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()

		auth := &recordingAuthorizer{}
		api := &API{
			Options: &Options{
				Logger: testutil.Logger(t),
			},
			HTTPAuth: &HTTPAuthorizer{
				Authorizer: auth,
				Logger:     testutil.Logger(t),
			},
		}
		ctx := dbauthz.As(t.Context(), actor) //nolint:gocritic // Testing authorization from the request context.

		require.True(t, api.canSubscribeUserSecretEvents(ctx, ownerID))
		require.Len(t, auth.calls, 1)
		require.Equal(t, actor, auth.calls[0].Actor)
		require.Equal(t, policy.ActionRead, auth.calls[0].Action)
		require.Equal(t, rbac.ResourceUserSecret.Type, auth.calls[0].Object.Type)
		require.Equal(t, ownerID.String(), auth.calls[0].Object.Owner)
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()

		auth := &recordingAuthorizer{err: xerrors.New("denied")}
		api := &API{
			Options: &Options{
				Logger: testutil.Logger(t),
			},
			HTTPAuth: &HTTPAuthorizer{
				Authorizer: auth,
				Logger:     testutil.Logger(t),
			},
		}
		ctx := dbauthz.As(t.Context(), actor) //nolint:gocritic // Testing authorization from the request context.

		require.False(t, api.canSubscribeUserSecretEvents(ctx, ownerID))
		require.Len(t, auth.calls, 1)
	})

	t.Run("no actor", func(t *testing.T) {
		t.Parallel()

		auth := &recordingAuthorizer{}
		logger := slogtest.Make(t, &slogtest.Options{
			IgnoredErrorIs: []error{},
			IgnoreErrorFn: func(entry slog.SinkEntry) bool {
				return entry.Message == "no authorization actor for user secret event subscription"
			},
		})
		api := &API{
			Options: &Options{
				Logger: logger,
			},
			HTTPAuth: &HTTPAuthorizer{
				Authorizer: auth,
				Logger:     logger,
			},
		}

		require.False(t, api.canSubscribeUserSecretEvents(context.Background(), ownerID))
		require.Empty(t, auth.calls)
	})
}

type recordingAuthorizer struct {
	err   error
	calls []rbac.AuthCall
}

func (a *recordingAuthorizer) Authorize(_ context.Context, subject rbac.Subject, action policy.Action, object rbac.Object) error {
	a.calls = append(a.calls, rbac.AuthCall{
		Actor:  subject,
		Action: action,
		Object: object,
	})
	return a.err
}

func (*recordingAuthorizer) Prepare(context.Context, rbac.Subject, policy.Action, string) (rbac.PreparedAuthorized, error) {
	//nolint:nilnil // Prepare is unused by these tests.
	return nil, nil
}
