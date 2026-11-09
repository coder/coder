package gitsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type acquiredChatDiffStatusesStore struct {
	Store
	rows []database.AcquireStaleChatDiffStatusesRow
}

func (s *acquiredChatDiffStatusesStore) AcquireStaleChatDiffStatuses(context.Context, database.AcquireStaleChatDiffStatusesParams) ([]database.AcquireStaleChatDiffStatusesRow, error) {
	return s.rows, nil
}

func TestWorker_RateLimitBackoff(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		header     string
		resetAfter time.Duration
		backoff    time.Duration
	}{
		{
			name:       "Primary",
			header:     "X-RateLimit-Reset",
			resetAfter: 30 * time.Minute,
			backoff:    35 * time.Minute,
		},
		{
			name:       "Secondary",
			header:     "Retry-After",
			resetAfter: 15 * time.Minute,
			backoff:    20 * time.Minute,
		},
		{
			name:       "FarFuture",
			header:     "X-RateLimit-Reset",
			resetAfter: 365 * 24 * time.Hour,
			backoff:    65 * time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db, _ := dbtestutil.NewDB(t)
			owner := dbgen.User(t, db, database.User{})
			otherOwner := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.ChatProvider(t, db, database.ChatProvider{})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
			chat := dbgen.Chat(t, db, database.Chat{
				OwnerID:           owner.ID,
				OrganizationID:    org.ID,
				LastModelConfigID: model.ID,
			})

			clock := quartz.NewMock(t)
			clock.Set(time.Now().UTC().Truncate(time.Second)).MustWait(ctx)
			start := clock.Now()
			retryAfter := start.Add(tc.backoff)
			var limitedCalls, otherCalls atomic.Int32
			var limited atomic.Bool
			limited.Store(true)

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "Bearer limited-token" {
					limitedCalls.Add(1)
					if limited.Load() {
						value := strconv.FormatInt(start.Add(tc.resetAfter).Unix(), 10)
						if tc.header == "Retry-After" {
							value = strconv.FormatInt(int64(tc.resetAfter/time.Second), 10)
						}
						w.Header().Set(tc.header, value)
						w.WriteHeader(http.StatusTooManyRequests)
						return
					}
				} else {
					otherCalls.Add(1)
				}
				_, _ = w.Write([]byte(`[]`))
			}))
			t.Cleanup(server.Close)

			provider, err := gitprovider.New("github", server.URL, server.Client(), gitprovider.WithClock(clock))
			require.NoError(t, err)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			newWorker := func(store Store) *Worker {
				refresher := NewRefresher(
					func(context.Context, string) gitprovider.Provider { return provider },
					func(_ context.Context, userID uuid.UUID, _ string) (*string, error) {
						if userID == owner.ID {
							return new("limited-token"), nil
						}
						return new("other-token"), nil
					},
					logger, clock, WithConcurrency(1),
				)
				return NewWorker(store, refresher, nil, clock, logger)
			}
			addRef := func(chatID uuid.UUID, repo, branch string) {
				_, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
					ChatID:          chatID,
					GitRemoteOrigin: server.URL + "/owner/" + repo,
					GitBranch:       branch,
					StaleAt:         start.Add(-time.Minute),
				})
				require.NoError(t, err)
			}
			for i := range 4 {
				addRef(chat.ID, "repo", fmt.Sprintf("branch-%d", i))
			}

			acquired, err := db.AcquireStaleChatDiffStatuses(ctx, database.AcquireStaleChatDiffStatusesParams{
				Now: start, LimitVal: 1,
			})
			require.NoError(t, err)
			require.Len(t, acquired, 1)
			require.Equal(t, start.Add(5*time.Minute), acquired[0].StaleAt.UTC(), "acquisition must retain its five-minute lease")

			worker := newWorker(db)
			worker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "two rows should be skipped in the same batch")

			// Another replica acquired this row before the cooldown was stored.
			queuedWorker := newWorker(&acquiredChatDiffStatusesStore{Store: db, rows: acquired})
			queuedWorker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "already acquired rows must recheck the cooldown")
			statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{chat.ID})
			require.NoError(t, err)
			require.Len(t, statuses, 4)
			for _, status := range statuses {
				assert.Equal(t, retryAfter, status.StaleAt.UTC(), "direct and skipped rows must use the same reset time")
			}

			// A shorter response from another replica must not shorten the cooldown.
			err = db.BackoffChatGitSync(ctx, database.BackoffChatGitSyncParams{
				OwnerID: owner.ID, RetryAfter: start.Add(10 * time.Minute),
			})
			require.NoError(t, err)

			// MarkStale and a new repository must not bypass the owner's cooldown.
			worker.MarkStale(ctx, MarkStaleParams{
				ChatID: chat.ID, Branch: "branch-0", Origin: server.URL + "/owner/repo",
			})
			addRef(chat.ID, "another-repo", "new-branch")
			otherChat := dbgen.Chat(t, db, database.Chat{
				OwnerID:           otherOwner.ID,
				OrganizationID:    org.ID,
				LastModelConfigID: model.ID,
			})
			addRef(otherChat.ID, "repo", "other-owner")

			// A separate worker has no in-memory state from the first tick.
			otherWorker := newWorker(db)
			clock.Set(retryAfter.Add(-time.Second)).MustWait(ctx)
			queuedWorker.tick(ctx)
			otherWorker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "no requests before reset, including on another replica")
			require.Equal(t, int32(1), otherCalls.Load(), "other owners should still refresh")

			limited.Store(false)
			clock.Advance(time.Second).MustWait(ctx)
			otherWorker.tick(ctx)
			require.Equal(t, int32(6), limitedCalls.Load(), "all five rows should refresh at reset")
		})
	}
}
