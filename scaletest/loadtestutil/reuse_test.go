package loadtestutil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/testutil"
)

// TestFilterScaletestUsersByPrefix covers the pure user-selection logic behind
// scaletest user reuse: an infix pool must not pick up users from the default
// pool (isolation), non-scaletest users are ignored, and the username guard
// rejects users that only match the prefix in another field.
func TestFilterScaletestUsersByPrefix(t *testing.T) {
	t.Parallel()

	users := []codersdk.User{
		scaletestUser("scaletest-notif-aaaaaaaa-0", "aaaaaaaa-0@scaletest.local"),
		scaletestUser("scaletest-notif-bbbbbbbb-1", "bbbbbbbb-1@scaletest.local"),
		// Default pool: a scaletest user that is NOT in the notif pool.
		scaletestUser("scaletest-cccccccc-0", "cccccccc-0@scaletest.local"),
		// Not a scaletest user at all.
		scaletestUser("regular-user", "regular@example.com"),
		// Scaletest email but a username that does not start with the prefix; the
		// guard must reject it even though a search could surface it.
		scaletestUser("admin", "scaletest-notif-dddddddd-9@scaletest.local"),
	}

	cases := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "infix isolates its own pool",
			prefix: "scaletest-notif-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
			},
		},
		{
			name:   "default prefix selects all scaletest users",
			prefix: loadtestutil.ScaleTestPrefix + "-",
			want: []string{
				"scaletest-notif-aaaaaaaa-0",
				"scaletest-notif-bbbbbbbb-1",
				"scaletest-cccccccc-0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := loadtestutil.FilterScaletestUsersByPrefix(users, tc.prefix)

			gotNames := make([]string, 0, len(got))
			for _, u := range got {
				gotNames = append(gotNames, u.Username)
			}
			require.ElementsMatch(t, tc.want, gotNames)
		})
	}
}

func TestReuseSearchPrefix(t *testing.T) {
	t.Parallel()

	require.Equal(t, "scaletest-", loadtestutil.ReuseSearchPrefix(""))
	require.Equal(t, "scaletest-notif-", loadtestutil.ReuseSearchPrefix("notif"))
}

func TestUserHasRole(t *testing.T) {
	t.Parallel()

	admin := codersdk.User{Roles: []codersdk.SlimRole{{Name: codersdk.RoleTemplateAdmin}}}
	require.True(t, loadtestutil.UserHasRole(admin, codersdk.RoleTemplateAdmin))
	require.False(t, loadtestutil.UserHasRole(codersdk.User{}, codersdk.RoleTemplateAdmin))
}

// TestSelectReuseUsers drives the selection success path with a hand-built pool:
// the filter predicate, the disjoint role split used by notifications, the
// matched[:count] slice, and the insufficient-pool error.
func TestSelectReuseUsers(t *testing.T) {
	t.Parallel()

	isTemplateAdmin := func(u codersdk.User) bool {
		return loadtestutil.UserHasRole(u, codersdk.RoleTemplateAdmin)
	}
	notTemplateAdmin := func(u codersdk.User) bool { return !isTemplateAdmin(u) }

	// Interleave admins and regulars so filtering order is exercised.
	admin1 := roleUser("scaletest-a1", codersdk.RoleTemplateAdmin)
	regular1 := roleUser("scaletest-r1")
	admin2 := roleUser("scaletest-a2", codersdk.RoleTemplateAdmin)
	regular2 := roleUser("scaletest-r2")
	pool := []codersdk.User{admin1, regular1, admin2, regular2}

	t.Run("RoleSplitIsDisjoint", func(t *testing.T) {
		t.Parallel()

		admins, err := loadtestutil.SelectReuseUsers(pool, 2, isTemplateAdmin)
		require.NoError(t, err)
		require.Equal(t, []codersdk.User{admin1, admin2}, admins)

		regulars, err := loadtestutil.SelectReuseUsers(pool, 2, notTemplateAdmin)
		require.NoError(t, err)
		require.Equal(t, []codersdk.User{regular1, regular2}, regulars)

		// The two groups never pick the same user.
		for _, a := range admins {
			for _, r := range regulars {
				require.NotEqual(t, a.ID, r.ID)
			}
		}
	})

	t.Run("NilFilterReturnsWholePool", func(t *testing.T) {
		t.Parallel()

		got, err := loadtestutil.SelectReuseUsers(pool, 4, nil)
		require.NoError(t, err)
		require.Equal(t, pool, got)
	})

	t.Run("SlicesToCount", func(t *testing.T) {
		t.Parallel()

		got, err := loadtestutil.SelectReuseUsers(pool, 1, isTemplateAdmin)
		require.NoError(t, err)
		require.Equal(t, []codersdk.User{admin1}, got)
	})

	t.Run("InsufficientUsers", func(t *testing.T) {
		t.Parallel()

		_, err := loadtestutil.SelectReuseUsers(pool, 3, isTemplateAdmin)
		var insufficient *loadtestutil.InsufficientUsersError
		require.ErrorAs(t, err, &insufficient)
		require.Equal(t, 2, insufficient.Found)
		require.Equal(t, 3, insufficient.Need)
		require.ErrorContains(t, err, "not enough scaletest users to reuse")
	})
}

// TestMintReuseTokens drives the minting success path against a stub of the
// single tokens endpoint: one token is minted per user, paired correctly, and
// the requested lifetime is forwarded.
func TestMintReuseTokens(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		lifetime = make(map[string]time.Duration)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// POST /api/v2/users/{id}/keys/tokens
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/users/"), "/keys/tokens")
		if r.Method != http.MethodPost || id == "" || id == r.URL.Path {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var req codersdk.CreateTokenRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		mu.Lock()
		lifetime[id] = req.Lifetime
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(codersdk.GenerateAPIKeyResponse{Key: "token-" + id}))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	client := codersdk.New(u)
	client.SetSessionToken("owner-token")

	users := []codersdk.User{roleUser("scaletest-a1"), roleUser("scaletest-a2")}

	ctx := testutil.Context(t, testutil.WaitShort)

	const want = 42 * time.Minute
	reuse, err := loadtestutil.MintReuseTokens(ctx, client, users, want)
	require.NoError(t, err)
	require.Len(t, reuse, len(users))
	for i, ru := range reuse {
		require.Equal(t, users[i], ru.User)
		require.Equal(t, "token-"+users[i].ID.String(), ru.SessionToken)
		mu.Lock()
		require.Equal(t, want, lifetime[users[i].ID.String()])
		mu.Unlock()
	}
}

func scaletestUser(username, email string) codersdk.User {
	return codersdk.User{
		ReducedUser: codersdk.ReducedUser{
			MinimalUser: codersdk.MinimalUser{ID: uuid.New(), Username: username},
			Email:       email,
		},
	}
}

func roleUser(username string, roles ...string) codersdk.User {
	u := scaletestUser(username, username+"@scaletest.local")
	for _, r := range roles {
		u.Roles = append(u.Roles, codersdk.SlimRole{Name: r})
	}
	return u
}
