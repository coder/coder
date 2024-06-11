package render_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestMacros(t *testing.T) {
	t.Parallel()

	const accessURL = "https://xyz.com"
	u, err := url.Parse("https://xyz.com")
	require.NoError(t, err)

	tests := []struct {
		name           string
		in             string
		cfg            codersdk.DeploymentValues
		expectedOutput string
		expectedErr    error
	}{
		{
			name: "ACCESS_URL",
			in:   "[ACCESS_URL]/workspaces",
			cfg: codersdk.DeploymentValues{
				AccessURL: *serpent.URLOf(u),
			},
			expectedOutput: accessURL + "/workspaces",
			expectedErr:    nil,
		},
		{
			name: "ACCESS_URL multiple",
			in:   "[ACCESS_URL] is [ACCESS_URL]",
			cfg: codersdk.DeploymentValues{
				AccessURL: *serpent.URLOf(u),
			},
			expectedOutput: accessURL + " is " + accessURL,
			expectedErr:    nil,
		},
	}

	for _, tc := range tests {
		tc := tc // unnecessary as of go1.22 but the linter is outdated

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := render.Macros(map[string]func() string{
				"ACCESS_URL": func() string {
					return accessURL
				},
			}, tc.in)
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.expectedErr)
			}

			require.Equal(t, tc.expectedOutput, out)
		})
	}
}
