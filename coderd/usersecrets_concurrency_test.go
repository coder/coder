package coderd_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestGetUserSecretByUserIDAndNameForUpdateLocks asserts that the query
// PATCH uses actually takes a row lock. A second transaction asking for the
// same row must block until the first one commits, which is what makes the
// handler's post-state check serialize instead of racing.
func TestGetUserSecretByUserIDAndNameForUpdateLocks(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	secret := dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  user.ID,
		Name:    "lock-me",
		EnvName: "LOCK_ME",
	})

	arg := database.GetUserSecretByUserIDAndNameForUpdateParams{
		UserID: user.ID,
		Name:   secret.Name,
	}

	var (
		locked   = make(chan struct{})
		release  = make(chan struct{})
		firstErr error
		wg       sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		firstErr = db.InTx(func(tx database.Store) error {
			if _, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		}, nil)
	}()

	testutil.TryReceive(ctx, t, locked)

	secondDone := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		secondDone <- db.InTx(func(tx database.Store) error {
			_, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg)
			return err
		}, nil)
	}()

	// The second transaction must not be able to take the lock while the
	// first one holds it.
	select {
	case err := <-secondDone:
		t.Fatalf("second transaction acquired the row lock while it was held: %v", err)
	case <-time.After(testutil.IntervalMedium):
	}

	close(release)
	require.NoError(t, testutil.TryReceive(ctx, t, secondDone))
	wg.Wait()
	require.NoError(t, firstErr)
}

// TestPatchUserSecretConcurrentTargetClears asserts that two PATCHes racing
// to clear the two different injection targets cannot both win. The loser
// must see the post-state produced by the winner and be rejected with a
// validation error rather than reaching an inconsistent row.
func TestPatchUserSecretConcurrentTargetClears(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	const name = "concurrent-clear"
	_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:     name,
		Value:    "value",
		EnvName:  "CONCURRENT_CLEAR",
		FilePath: "/tmp/concurrent-clear",
	})
	require.NoError(t, err)

	empty := ""
	reqs := []codersdk.UpdateUserSecretRequest{
		{EnvName: &empty},
		{FilePath: &empty},
	}

	var (
		start = make(chan struct{})
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []error
	)
	for _, req := range reqs {
		wg.Add(1)
		go func(req codersdk.UpdateUserSecretRequest) {
			defer wg.Done()
			<-start
			_, err := client.UpdateUserSecret(ctx, codersdk.Me, name, req)
			mu.Lock()
			defer mu.Unlock()
			errs = append(errs, err)
		}(req)
	}
	close(start)
	wg.Wait()

	var succeeded int
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		// The loser must fail validation, never with a server error.
		requireSecretValidationContainsError(t, err, http.StatusBadRequest,
			"env_name", "at least one of env_name or file_path")
	}
	assert.Equal(t, 1, succeeded, "exactly one concurrent PATCH may clear a target")

	// The surviving row still has exactly one target.
	final, err := client.UserSecretByName(ctx, codersdk.Me, name)
	require.NoError(t, err)
	assert.True(t, (final.EnvName == "") != (final.FilePath == ""), "row must keep exactly one target")
	assert.True(t, final.Enabled)
}
