package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

// TestFilterScaletestUsersByPrefix covers the pure user-selection logic behind
// notifications --reuse-users: a sub-prefix pool must not pick up users from the
// default pool (isolation), non-scaletest users are ignored, and the username
// guard rejects users that only match the prefix in another field.
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
			name:   "sub-prefix isolates its own pool",
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

			got := filterScaletestUsersByPrefix(users, tc.prefix)

			gotNames := make([]string, 0, len(got))
			for _, u := range got {
				gotNames = append(gotNames, u.Username)
			}
			require.ElementsMatch(t, tc.want, gotNames)
		})
	}
}

func scaletestUser(username, email string) codersdk.User {
	return codersdk.User{
		ReducedUser: codersdk.ReducedUser{
			MinimalUser: codersdk.MinimalUser{Username: username},
			Email:       email,
		},
	}
}
