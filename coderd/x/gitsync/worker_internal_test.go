package gitsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type acquiredChatDiffStatusesStore struct {
	Store
	rows []database.AcquireStaleChatDiffStatusesRow
}

func (s *acquiredChatDiffStatusesStore) AcquireStaleChatDiffStatuses(context.Context, int32) ([]database.AcquireStaleChatDiffStatusesRow, error) {
	return s.rows, nil
}

type cooldownStoreSpy struct {
	Store
	reads, writes int
	owners        []uuid.UUID
}

func (s *cooldownStoreSpy) GetChatGitSyncRateLimitsByUserIDs(ctx context.Context, owners []uuid.UUID) ([]database.ChatGitSyncRateLimit, error) {
	s.reads++
	s.owners = append([]uuid.UUID{}, owners...)
	return s.Store.GetChatGitSyncRateLimitsByUserIDs(ctx, owners)
}

func (s *cooldownStoreSpy) UpsertChatGitSyncRateLimit(ctx context.Context, arg database.UpsertChatGitSyncRateLimitParams) error {
	s.writes++
	return s.Store.UpsertChatGitSyncRateLimit(ctx, arg)
}

func TestWorker_CooldownLookupError(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	store := dbmock.NewMockStore(gomock.NewController(t))
	store.EXPECT().AcquireStaleChatDiffStatuses(gomock.Any(), gomock.Any()).Return([]database.AcquireStaleChatDiffStatusesRow{{
		ChatID:          uuid.New(),
		OwnerID:         uuid.New(),
		GitRemoteOrigin: "https://github.com/owner/repo",
		GitBranch:       "main",
	}}, nil)
	store.EXPECT().GetChatGitSyncRateLimitsByUserIDs(gomock.Any(), gomock.Any()).Return(nil, xerrors.New("database unavailable"))
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	refresher := NewRefresher(func(context.Context, string) gitprovider.Provider {
		t.Error("provider resolution must not run when cooldown lookup fails")
		return nil
	}, nil, logger, clock)
	worker := NewWorker(store, refresher, nil, clock, logger)
	worker.tick(testutil.Context(t, testutil.WaitShort))
}

func TestWorker_Cooldown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		header     string
		resetAfter time.Duration
		cooldown   time.Duration
	}{
		{
			name:       "PrimaryRateLimit",
			header:     "X-RateLimit-Reset",
			resetAfter: 30 * time.Minute,
			cooldown:   35 * time.Minute,
		},
		{
			name:       "SecondaryRateLimit",
			header:     "Retry-After",
			resetAfter: 15 * time.Minute,
			cooldown:   20 * time.Minute,
		},
		{
			name:       "FarFutureReset",
			header:     "X-RateLimit-Reset",
			resetAfter: 365 * 24 * time.Hour,
			cooldown:   65 * time.Minute,
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

			// Keep scheduling timestamps before PostgreSQL NOW() so acquisition
			// stays eligible while the worker clock controls cooldown expiry.
			clock := quartz.NewMock(t)
			clock.Set(time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)).MustWait(ctx)
			start := clock.Now()
			retryAfter := start.Add(tc.cooldown)
			var limitedCalls, otherCalls, enterpriseCalls, gitlabCalls atomic.Int32
			var limited atomic.Bool
			limited.Store(true)

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Host {
				case "ghe.example.com":
					enterpriseCalls.Add(1)
				case "gitlab.com":
					gitlabCalls.Add(1)
				default:
					if r.Header.Get("Authorization") == "Bearer limited-token" {
						limitedCalls.Add(1)
						if limited.Load() {
							value := strconv.FormatInt(start.Add(tc.resetAfter).Unix(), 10)
							remaining := "0"
							if tc.header == "Retry-After" {
								value = strconv.FormatInt(int64(tc.resetAfter/time.Second), 10)
								remaining = "4990"
							}
							w.Header().Set(tc.header, value)
							w.Header().Set("X-RateLimit-Remaining", remaining)
							w.WriteHeader(http.StatusTooManyRequests)
							return
						}
					} else {
						otherCalls.Add(1)
					}
				}
				_, _ = w.Write([]byte(`[]`))
			}))
			t.Cleanup(server.Close)

			// Every provider request goes to the local server, never its API host.
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := server.Client()
			transport := client.Transport
			client.Transport = testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				r = r.Clone(r.Context())
				r.URL.Scheme = target.Scheme
				r.URL.Host = target.Host
				return transport.RoundTrip(r)
			})
			github, err := gitprovider.New("github", "", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			enterprise, err := gitprovider.New("github", "https://ghe.example.com/api/v3", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			gitlab, err := gitprovider.New("gitlab", "https://gitlab.com/api/v4", client, gitprovider.WithClock(clock))
			require.NoError(t, err)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			newWorker := func(store Store) *Worker {
				refresher := NewRefresher(
					func(_ context.Context, origin string) gitprovider.Provider {
						switch {
						case strings.Contains(origin, "ghe.example.com"):
							return enterprise
						case strings.Contains(origin, "gitlab.com"):
							return gitlab
						default:
							return github
						}
					},
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
			addRef := func(chatID uuid.UUID, origin, branch string) {
				_, err := db.UpsertChatDiffStatusReference(ctx, database.UpsertChatDiffStatusReferenceParams{
					ChatID:          chatID,
					GitRemoteOrigin: origin,
					GitBranch:       branch,
					StaleAt:         start.Add(-time.Minute),
				})
				require.NoError(t, err)
			}
			for i := range 4 {
				addRef(chat.ID, "https://github.com/owner/repo", fmt.Sprintf("branch-%d", i))
			}
			acquired, err := db.AcquireStaleChatDiffStatuses(ctx, 1)
			require.NoError(t, err)
			require.Len(t, acquired, 1)

			store := &cooldownStoreSpy{Store: db}
			worker := newWorker(store)
			worker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "two rows should be skipped in the same batch")
			require.Equal(t, 1, store.reads, "one read per tick, not per row")
			require.Equal(t, []uuid.UUID{owner.ID}, store.owners)
			require.Equal(t, 1, store.writes, "one write per limited account, not per error")
			limits, err := db.GetChatGitSyncRateLimitsByUserIDs(ctx, []uuid.UUID{owner.ID})
			require.NoError(t, err)
			require.Len(t, limits, 1)
			require.Equal(t, "github.com", limits[0].Host)
			require.Equal(t, retryAfter, limits[0].RetryAfter.UTC())

			// The worker must check a row acquired before the cooldown was stored.
			queuedStore := &cooldownStoreSpy{Store: db}
			queuedWorker := newWorker(&acquiredChatDiffStatusesStore{Store: queuedStore, rows: acquired})
			queuedWorker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load())
			require.Equal(t, 0, queuedStore.writes, "reading a cooldown must not write it back")
			statuses, err := db.GetChatDiffStatusesByChatIDs(ctx, []uuid.UUID{chat.ID})
			require.NoError(t, err)
			require.Len(t, statuses, 4)
			for _, status := range statuses {
				assert.Equal(t, retryAfter, status.StaleAt.UTC(), "direct, in-batch and cooldown-skipped rows share the reset")
			}

			err = db.UpsertChatGitSyncRateLimit(ctx, database.UpsertChatGitSyncRateLimitParams{
				UserID:     owner.ID,
				Host:       "github.com",
				RetryAfter: start.Add(10 * time.Minute),
			})
			require.NoError(t, err)
			worker.MarkStale(ctx, MarkStaleParams{
				ChatID: chat.ID,
				Branch: "branch-0",
				Origin: "https://github.com/owner/repo",
			})
			addRef(chat.ID, "https://github.com/owner/another-repo", "new-branch")
			addRef(chat.ID, "git@github.com:owner/repo.git", "ssh-branch")
			addRef(chat.ID, "https://ghe.example.com/owner/repo", "enterprise")
			addRef(chat.ID, "https://gitlab.com/owner/repo", "gitlab")
			otherChat := dbgen.Chat(t, db, database.Chat{
				OwnerID:           otherOwner.ID,
				OrganizationID:    org.ID,
				LastModelConfigID: model.ID,
			})
			addRef(otherChat.ID, "https://github.com/owner/repo", "other-owner")

			otherStore := &cooldownStoreSpy{Store: db}
			otherWorker := newWorker(otherStore)
			clock.Set(retryAfter.Add(-time.Second)).MustWait(ctx)
			queuedWorker.tick(ctx)
			otherWorker.tick(ctx)
			require.Equal(t, int32(1), limitedCalls.Load(), "no request before reset, including SSH and new refs")
			require.Equal(t, int32(1), otherCalls.Load())
			require.Equal(t, int32(1), enterpriseCalls.Load(), "Enterprise diff-status refresh should not be in cooldown")
			require.Equal(t, int32(1), gitlabCalls.Load(), "GitLab diff-status refresh should not be in cooldown")
			require.Equal(t, 1, otherStore.reads)
			require.ElementsMatch(t, []uuid.UUID{owner.ID, otherOwner.ID}, otherStore.owners)
			require.Equal(t, 0, otherStore.writes)

			limited.Store(false)
			clock.Advance(time.Second).MustWait(ctx)
			otherWorker.tick(ctx)
			require.Equal(t, int32(7), limitedCalls.Load(), "all six affected rows should retry at reset")
			require.Equal(t, 0, otherStore.writes)
		})
	}
}
