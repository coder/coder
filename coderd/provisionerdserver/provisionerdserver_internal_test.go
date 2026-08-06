package provisionerdserver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestShouldRefreshOIDCToken(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	testCases := []struct {
		name string
		link database.UserLink
		want bool
	}{
		{
			name: "NoRefreshToken",
			link: database.UserLink{OAuthExpiry: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "ZeroExpiry",
			link: database.UserLink{OAuthRefreshToken: "refresh"},
			want: false,
		},
		{
			name: "LongExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Hour),
			},
			want: true,
		},
		{
			// Edge being "+/- 10 minutes"
			name: "EdgeExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Minute * 10),
			},
			want: true,
		},
		{
			name: "Expired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Minute),
			},
			want: true,
		},
		{
			name: "SoonToBeExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(5 * time.Minute),
			},
			want: true,
		},
		{
			name: "SoonToBeExpiredEdge",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(9 * time.Minute),
			},
			want: true,
		},
		{
			name: "AfterEdge",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(11 * time.Minute),
			},
			want: false,
		},
		{
			name: "NotExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour),
			},
			want: false,
		},
		{
			name: "NotEvenCloseExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour * 24),
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			shouldRefresh, _ := shouldRefreshOIDCToken(tc.link)
			require.Equal(t, tc.want, shouldRefresh)
		})
	}
}

func TestObtainOIDCAccessToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, nil, uuid.Nil)
		require.NoError(t, err)
	})
	t.Run("InvalidConfig", func(t *testing.T) {
		// We still want OIDC to succeed even if exchanging the token fails.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &oauth2.Config{}, user.ID)
		require.NoError(t, err)
	})
	t.Run("MissingLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
		})
		tok, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &oauth2.Config{}, user.ID)
		require.Empty(t, tok)
		require.NoError(t, err)
	})
	t.Run("Exchange", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &testutil.OAuth2Config{
			Token: &oauth2.Token{
				AccessToken: "token",
			},
		}, user.ID)
		require.NoError(t, err)
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "token", link.OAuthAccessToken)
	})
}

// TestNewServer_SessionCancelRequired verifies that constructing a server for
// a deletable provisioner key without a SessionCancel fails, while reserved
// keys do not require one.
func TestNewServer_SessionCancelRequired(t *testing.T) {
	t.Parallel()

	// The SessionCancel validation runs before the remaining nil-pointer
	// checks, so the other arguments can be zero values.
	newServer := func(keyID uuid.UUID) error {
		_, err := NewServer(
			context.Background(), "", nil, uuid.Nil, uuid.Nil, slog.Logger{},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			Options{KeyID: keyID},
			nil, nil, nil, codersdk.Experiments{},
		)
		return err
	}

	require.ErrorContains(t, newServer(uuid.New()), "SessionCancel is required")
	// A reserved key passes the SessionCancel check; the error comes from the
	// next validation instead.
	require.ErrorContains(t, newServer(codersdk.ProvisionerKeyUUIDPSK), "quotaCommitter is nil")
}

// TestTerminateSession_Deferral verifies that session cancellation is
// immediate when no job is active and deferred until the last active job
// finishes otherwise.
func TestTerminateSession_Deferral(t *testing.T) {
	t.Parallel()

	newTestServer := func(canceled chan struct{}) *server {
		return &server{
			lifecycleCtx:  context.Background(),
			Logger:        testutil.Logger(t),
			sessionCancel: func() { close(canceled) },
			activeJobs:    map[uuid.UUID]struct{}{},
		}
	}
	assertCanceled := func(t *testing.T, canceled chan struct{}, want bool) {
		t.Helper()
		select {
		case <-canceled:
			require.True(t, want, "session canceled unexpectedly")
		default:
			require.False(t, want, "expected session to be canceled")
		}
	}

	t.Run("ImmediateWhenIdle", func(t *testing.T) {
		t.Parallel()
		canceled := make(chan struct{})
		s := newTestServer(canceled)
		s.TerminateSession()
		assertCanceled(t, canceled, true)
	})

	t.Run("DeferredUntilJobsFinish", func(t *testing.T) {
		t.Parallel()
		canceled := make(chan struct{})
		s := newTestServer(canceled)
		job1, job2 := uuid.New(), uuid.New()
		s.jobStarted(job1)
		s.jobStarted(job2)

		s.TerminateSession()
		assertCanceled(t, canceled, false)

		s.jobFinished(job1)
		assertCanceled(t, canceled, false)

		s.jobFinished(job2)
		assertCanceled(t, canceled, true)
	})

	t.Run("NoPendingTerminationNoCancel", func(t *testing.T) {
		t.Parallel()
		canceled := make(chan struct{})
		s := newTestServer(canceled)
		jobID := uuid.New()
		s.jobStarted(jobID)
		s.jobFinished(jobID)
		assertCanceled(t, canceled, false)
	})
}
