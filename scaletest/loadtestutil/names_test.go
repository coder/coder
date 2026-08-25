package loadtestutil_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

func TestGenerateUserIdentifierWithPrefix(t *testing.T) {
	t.Parallel()

	const id = "7"

	cases := []struct {
		name       string
		prefix     string
		wantPrefix string
	}{
		{
			name:       "default scaletest prefix",
			prefix:     loadtestutil.ScaleTestPrefix + "-",
			wantPrefix: "scaletest-",
		},
		{
			name:       "sub-prefix inserted after scaletest root",
			prefix:     loadtestutil.ScaleTestPrefix + "-notif-",
			wantPrefix: "scaletest-notif-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			username, email, err := loadtestutil.GenerateUserIdentifierWithPrefix(tc.prefix, id)
			require.NoError(t, err)

			// username == <prefix><random>-<id>.
			require.True(t, strings.HasPrefix(username, tc.wantPrefix),
				"username %q should start with %q", username, tc.wantPrefix)
			require.True(t, strings.HasSuffix(username, "-"+id),
				"username %q should end with %q", username, "-"+id)

			randPart := strings.TrimSuffix(strings.TrimPrefix(username, tc.prefix), "-"+id)
			require.Len(t, randPart, loadtestutil.DefaultRandLength)

			// The email reuses the same random-id and keeps the scaletest domain.
			require.Equal(t, randPart+"-"+id+loadtestutil.EmailDomain, email)

			// Custom prefixes that keep the scaletest- root stay discoverable.
			require.True(t, loadtestutil.IsScaleTestUser(username, email))
		})
	}
}

func TestGenerateUserIdentifierUsesDefaultPrefix(t *testing.T) {
	t.Parallel()

	username, email, err := loadtestutil.GenerateUserIdentifier("3")
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(username, loadtestutil.ScaleTestPrefix+"-"),
		"username %q should start with the scaletest- root", username)
	require.True(t, strings.HasSuffix(username, "-3"),
		"username %q should end with the id", username)
	require.True(t, strings.HasSuffix(email, "-3"+loadtestutil.EmailDomain),
		"email %q should end with the id and scaletest domain", email)
	require.True(t, loadtestutil.IsScaleTestUser(username, email))
}
