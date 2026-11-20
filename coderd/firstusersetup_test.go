package coderd_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v43/github"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const concurrentFirstUserRequests = 10

func firstUserRequest(i int) codersdk.CreateFirstUserRequest {
	return codersdk.CreateFirstUserRequest{
		Email:    fmt.Sprintf("user%d@coder.com", i),
		Username: fmt.Sprintf("user%d", i),
		Password: "SomeSecurePassword!",
	}
}

// githubSignupConfig makes every GitHub callback a different GitHub
// user, so that concurrent callbacks are sign-ups by different people.
func githubSignupConfig(allowSignups bool) *coderd.GithubOAuth2Config {
	var users, emails atomic.Int64
	return &coderd.GithubOAuth2Config{
		OAuth2Config:  &testutil.OAuth2Config{},
		AllowSignups:  allowSignups,
		AllowEveryone: true,
		AuthenticatedUser: func(context.Context, *http.Client) (*github.User, error) {
			id := users.Add(1)
			return &github.User{
				ID:    github.Int64(id),
				Login: github.String(fmt.Sprintf("github%d", id)),
			}, nil
		},
		ListEmails: func(context.Context, *http.Client) ([]*github.UserEmail, error) {
			return []*github.UserEmail{{
				Email:    github.String(fmt.Sprintf("github%d@coder.com", emails.Add(1))),
				Verified: github.Bool(true),
				Primary:  github.Bool(true),
			}}, nil
		},
	}
}

// githubCallbackStatus completes a GitHub sign-in and returns the HTTP
// status. A sign-in that succeeds is redirected.
func githubCallbackStatus(ctx context.Context, serverURL *url.URL) (int, error) {
	const state = "somestate"
	callbackURL, err := serverURL.Parse("/api/v2/users/oauth2/github/callback?code=asd&state=" + state)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL.String(), nil)
	if err != nil {
		return 0, err
	}
	req.AddCookie(&http.Cookie{Name: codersdk.OAuth2StateCookie, Value: state})
	httpClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = res.Body.Close()
	return res.StatusCode, nil
}

func createFirstUserStatus(ctx context.Context, client *codersdk.Client, req codersdk.CreateFirstUserRequest) (int, error) {
	_, err := client.CreateFirstUser(ctx, req)
	if err == nil {
		return http.StatusCreated, nil
	}
	if apiErr, ok := codersdk.AsError(err); ok {
		return apiErr.StatusCode(), nil
	}
	return 0, err
}

func requireOneOwner(ctx context.Context, t *testing.T, db database.Store) {
	t.Helper()
	//nolint:gocritic // The test reads all users.
	owners, err := db.GetUsers(dbauthz.AsSystemRestricted(ctx), database.GetUsersParams{
		RbacRole: []string{rbac.RoleOwner().String()},
	})
	require.NoError(t, err)
	require.Len(t, owners, 1)
}

// holdFirstUserSetupLock takes the lock in a transaction of its own and
// keeps it until the returned function is called.
func holdFirstUserSetupLock(ctx context.Context, t *testing.T, db database.Store) (release func()) {
	t.Helper()
	held := make(chan struct{})
	unlock := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.InTx(func(tx database.Store) error {
			if err := tx.AcquireLock(ctx, database.LockIDFirstUserSetup); err != nil {
				return err
			}
			close(held)
			<-unlock
			return nil
		}, nil)
	}()
	select {
	case <-held:
	case err := <-done:
		require.NoError(t, err)
		t.Fatal("transaction ended before the lock was held")
	case <-ctx.Done():
		t.Fatal("timed out taking the lock")
	}
	var once sync.Once
	unlockOnce := func() { once.Do(func() { close(unlock) }) }
	// A failed test must still let go of the lock, or a request that
	// waits on it would keep the server from shutting down.
	t.Cleanup(unlockOnce)
	return func() {
		unlockOnce()
		require.NoError(t, testutil.TryReceive(ctx, t, done))
	}
}

// setDefaultRepeatableRead makes repeatable read the default isolation
// level of this test's database.
func setDefaultRepeatableRead(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var name string
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT current_database()").Scan(&name))
	_, err := sqlDB.ExecContext(ctx, "ALTER DATABASE "+pq.QuoteIdentifier(name)+" SET default_transaction_isolation = 'repeatable read'")
	require.NoError(t, err)
	// The setting applies to new connections only, so stop reusing
	// the connections that were opened before it.
	sqlDB.SetMaxIdleConns(0)
}

// requireLockWaiters waits until Postgres reports the given number of
// requests waiting for an advisory lock in this test's database.
func requireLockWaiters(ctx context.Context, t *testing.T, sqlDB *sql.DB, want int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waiting int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		`).Scan(&waiting)
		return err == nil && waiting == want
	}, testutil.IntervalFast)
}

func TestFirstUserSetup_ConcurrentPassword(t *testing.T) {
	t.Parallel()
	fTelemetry := newFakeTelemetryReporter(testutil.Context(t, testutil.WaitSuperLong), t, concurrentFirstUserRequests)
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		TelemetryReporter: fTelemetry,
	})
	ctx := testutil.Context(t, testutil.WaitLong)

	type result struct {
		status int
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, concurrentFirstUserRequests)
	for i := range concurrentFirstUserRequests {
		go func() {
			<-start
			status, err := createFirstUserStatus(ctx, client, firstUserRequest(i))
			results <- result{status: status, err: err}
		}()
	}
	close(start)

	statuses := map[int]int{}
	for range concurrentFirstUserRequests {
		res := testutil.TryReceive(ctx, t, results)
		require.NoError(t, res.err)
		statuses[res.status]++
	}
	require.Equal(t, map[int]int{
		http.StatusCreated:  1,
		http.StatusConflict: concurrentFirstUserRequests - 1,
	}, statuses)

	//nolint:gocritic // The test reads the user count.
	userCount, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
	require.NoError(t, err)
	require.EqualValues(t, 1, userCount)
	requireOneOwner(ctx, t, db)

	// Every request has returned, so every report has been made.
	require.Len(t, fTelemetry.snapshots, 1)
}

func TestFirstUserSetup_ConcurrentOAuth(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		allowSignups bool
		// wantUsers is the number of users after all sign-ins.
		wantUsers int64
		// wantOther is the status of every sign-in but the first.
		wantOther int
	}{
		{name: "SignupsDisabled", allowSignups: false, wantUsers: 1, wantOther: http.StatusForbidden},
		{name: "SignupsEnabled", allowSignups: true, wantUsers: concurrentFirstUserRequests, wantOther: http.StatusTemporaryRedirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
				GithubOAuth2Config: githubSignupConfig(tc.allowSignups),
			})
			ctx := testutil.Context(t, testutil.WaitLong)

			type result struct {
				status int
				err    error
			}
			start := make(chan struct{})
			results := make(chan result, concurrentFirstUserRequests)
			for range concurrentFirstUserRequests {
				go func() {
					<-start
					status, err := githubCallbackStatus(ctx, client.URL)
					results <- result{status: status, err: err}
				}()
			}
			close(start)

			statuses := map[int]int{}
			for range concurrentFirstUserRequests {
				res := testutil.TryReceive(ctx, t, results)
				require.NoError(t, res.err)
				statuses[res.status]++
			}
			want := map[int]int{http.StatusTemporaryRedirect: 1}
			want[tc.wantOther] += concurrentFirstUserRequests - 1
			require.Equal(t, want, statuses)

			//nolint:gocritic // The test reads the user count.
			userCount, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
			require.NoError(t, err)
			require.Equal(t, tc.wantUsers, userCount)
			requireOneOwner(ctx, t, db)
		})
	}
}

func TestFirstUserSetup_PasswordAndOAuth(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		GithubOAuth2Config: githubSignupConfig(true),
	})
	ctx := testutil.Context(t, testutil.WaitLong)

	// Half of the requests use each path, so that the two paths are
	// likely to overlap.
	start := make(chan struct{})
	done := make(chan error, concurrentFirstUserRequests)
	for i := range concurrentFirstUserRequests {
		go func() {
			<-start
			if i%2 == 0 {
				_, err := createFirstUserStatus(ctx, client, firstUserRequest(i))
				done <- err
				return
			}
			_, err := githubCallbackStatus(ctx, client.URL)
			done <- err
		}()
	}
	close(start)
	for range concurrentFirstUserRequests {
		require.NoError(t, testutil.TryReceive(ctx, t, done))
	}

	requireOneOwner(ctx, t, db)
}

func TestFirstUserSetup_Lock(t *testing.T) {
	t.Parallel()

	t.Run("RequestWaitsForLock", func(t *testing.T) {
		t.Parallel()
		db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		ctx := testutil.Context(t, testutil.WaitLong)
		release := holdFirstUserSetupLock(ctx, t, db)

		status := make(chan int, 1)
		go func() {
			got, _ := createFirstUserStatus(ctx, client, coderdtest.FirstUserParams)
			status <- got
		}()

		requireLockWaiters(ctx, t, sqlDB, 1)
		select {
		case got := <-status:
			t.Fatalf("request returned %d while the lock was held", got)
		default:
		}

		release()
		require.Equal(t, http.StatusCreated, testutil.TryReceive(ctx, t, status))
	})

	// A Postgres server can make repeatable read its default. The count
	// under the lock must still see a user committed during the wait.
	t.Run("ServerDefaultRepeatableRead", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name string
			// request returns the status of one first user attempt.
			request    func(ctx context.Context, client *codersdk.Client) (int, error)
			wantStatus int
		}{
			{
				name: "Password",
				request: func(ctx context.Context, client *codersdk.Client) (int, error) {
					return createFirstUserStatus(ctx, client, firstUserRequest(1))
				},
				wantStatus: http.StatusConflict,
			},
			{
				name: "OAuth",
				request: func(ctx context.Context, client *codersdk.Client) (int, error) {
					return githubCallbackStatus(ctx, client.URL)
				},
				wantStatus: http.StatusForbidden,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
				setDefaultRepeatableRead(t, sqlDB)
				client := coderdtest.New(t, &coderdtest.Options{
					Database:           db,
					Pubsub:             ps,
					GithubOAuth2Config: githubSignupConfig(false),
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				release := holdFirstUserSetupLock(ctx, t, db)

				type result struct {
					status int
					err    error
				}
				results := make(chan result, 1)
				go func() {
					status, err := tc.request(ctx, client)
					results <- result{status: status, err: err}
				}()

				requireLockWaiters(ctx, t, sqlDB, 1)
				dbgen.User(t, db, database.User{})
				release()

				res := testutil.TryReceive(ctx, t, results)
				require.NoError(t, res.err)
				require.Equal(t, tc.wantStatus, res.status)
				//nolint:gocritic // The test reads the user count.
				userCount, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
				require.NoError(t, err)
				require.EqualValues(t, 1, userCount)
			})
		}
	})

	t.Run("ClientGivesUp", func(t *testing.T) {
		t.Parallel()
		db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		ctx := testutil.Context(t, testutil.WaitLong)
		release := holdFirstUserSetupLock(ctx, t, db)

		reqCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		reqErr := make(chan error, 1)
		go func() {
			_, err := client.CreateFirstUser(reqCtx, coderdtest.FirstUserParams)
			reqErr <- err
		}()

		requireLockWaiters(ctx, t, sqlDB, 1)
		cancel()
		require.ErrorIs(t, testutil.TryReceive(ctx, t, reqErr), context.Canceled)
		// The server must stop waiting too, or the request would create
		// the user after the client has gone.
		requireLockWaiters(ctx, t, sqlDB, 0)

		release()
		//nolint:gocritic // The test reads the user count.
		userCount, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
		require.NoError(t, err)
		require.Zero(t, userCount)
	})

	t.Run("TrialRequestedBeforeLock", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		trialRequested := make(chan struct{}, 1)
		client := coderdtest.New(t, &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			TrialGenerator: func(context.Context, codersdk.LicensorTrialRequest) error {
				trialRequested <- struct{}{}
				return nil
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		release := holdFirstUserSetupLock(ctx, t, db)

		req := coderdtest.FirstUserParams
		req.Trial = true
		status := make(chan int, 1)
		go func() {
			got, _ := createFirstUserStatus(ctx, client, req)
			status <- got
		}()

		// The trial request is a network call in production, so it must
		// not be made while the lock is held.
		testutil.TryReceive(ctx, t, trialRequested)

		release()
		require.Equal(t, http.StatusCreated, testutil.TryReceive(ctx, t, status))
	})

	t.Run("NotTakenWhenUsersExist", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database:           db,
			Pubsub:             ps,
			GithubOAuth2Config: githubSignupConfig(true),
		})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		release := holdFirstUserSetupLock(ctx, t, db)

		// Neither request returns before the test times out if it asks
		// for the lock.
		status, err := createFirstUserStatus(ctx, codersdk.New(client.URL), firstUserRequest(1))
		require.NoError(t, err)
		require.Equal(t, http.StatusConflict, status)

		status, err = githubCallbackStatus(ctx, client.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusTemporaryRedirect, status)
		release()
	})
}
