package dbmem_test

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// test that transactions don't deadlock, and that we don't see intermediate state.
func TestInTx(t *testing.T) {
	t.Parallel()

	uut := dbmem.New()

	inTx := make(chan any)
	queriesDone := make(chan any)
	queriesStarted := make(chan any)
	go func() {
		err := uut.InTx(func(tx database.Store) error {
			close(inTx)
			_, err := tx.InsertOrganization(context.Background(), database.InsertOrganizationParams{
				Name: "1",
			})
			assert.NoError(t, err)
			<-queriesStarted
			time.Sleep(5 * time.Millisecond)
			_, err = tx.InsertOrganization(context.Background(), database.InsertOrganizationParams{
				Name: "2",
			})
			assert.NoError(t, err)
			return nil
		}, nil)
		assert.NoError(t, err)
	}()
	var nums []int
	go func() {
		<-inTx
		for i := 0; i < 20; i++ {
			orgs, err := uut.GetOrganizations(context.Background())
			if err != nil {
				assert.ErrorIs(t, err, sql.ErrNoRows)
			}
			nums = append(nums, len(orgs))
			time.Sleep(time.Millisecond)
		}
		close(queriesDone)
	}()
	close(queriesStarted)
	<-queriesDone
	// ensure we never saw 1 org, only 0 or 2.
	for i := 0; i < 20; i++ {
		assert.NotEqual(t, 1, nums[i])
	}
}

// TestUserOrder ensures that the fake database returns users sorted by username.
func TestUserOrder(t *testing.T) {
	t.Parallel()

	db := dbmem.New()
	now := dbtime.Now()

	usernames := []string{"b-user", "d-user", "a-user", "c-user", "e-user"}
	for _, username := range usernames {
		dbgen.User(t, db, database.User{Username: username, CreatedAt: now})
	}

	users, err := db.GetUsers(context.Background(), database.GetUsersParams{})
	require.NoError(t, err)
	require.Lenf(t, users, len(usernames), "expected %d users", len(usernames))

	sort.Strings(usernames)
	for i, user := range users {
		require.Equal(t, usernames[i], user.Username)
	}
}

func TestProxyByHostname(t *testing.T) {
	t.Parallel()

	db := dbmem.New()

	// Insert a bunch of different proxies.
	proxies := []struct {
		name             string
		accessURL        string
		wildcardHostname string
	}{
		{
			name:             "one",
			accessURL:        "https://one.coder.com",
			wildcardHostname: "*.wildcard.one.coder.com",
		},
		{
			name:             "two",
			accessURL:        "https://two.coder.com",
			wildcardHostname: "*--suffix.two.coder.com",
		},
	}
	for _, p := range proxies {
		dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{
			Name:             p.name,
			Url:              p.accessURL,
			WildcardHostname: p.wildcardHostname,
		})
	}

	cases := []struct {
		name              string
		testHostname      string
		allowAccessURL    bool
		allowWildcardHost bool
		matchProxyName    string
	}{
		{
			name:              "NoMatch",
			testHostname:      "test.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "MatchAccessURL",
			testHostname:      "one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "one",
		},
		{
			name:              "MatchWildcard",
			testHostname:      "something.wildcard.one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "one",
		},
		{
			name:              "MatchSuffix",
			testHostname:      "something--suffix.two.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "two",
		},
		{
			name:              "ValidateHostname/1",
			testHostname:      ".*ne.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "ValidateHostname/2",
			testHostname:      "https://one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "ValidateHostname/3",
			testHostname:      "one.coder.com:8080/hello",
			allowAccessURL:    true,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "IgnoreAccessURLMatch",
			testHostname:      "one.coder.com",
			allowAccessURL:    false,
			allowWildcardHost: true,
			matchProxyName:    "",
		},
		{
			name:              "IgnoreWildcardMatch",
			testHostname:      "hi.wildcard.one.coder.com",
			allowAccessURL:    true,
			allowWildcardHost: false,
			matchProxyName:    "",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			proxy, err := db.GetWorkspaceProxyByHostname(context.Background(), database.GetWorkspaceProxyByHostnameParams{
				Hostname:              c.testHostname,
				AllowAccessUrl:        c.allowAccessURL,
				AllowWildcardHostname: c.allowWildcardHost,
			})
			if c.matchProxyName == "" {
				require.ErrorIs(t, err, sql.ErrNoRows)
				require.Empty(t, proxy)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, proxy)
				require.Equal(t, c.matchProxyName, proxy.Name)
			}
		})
	}
}
