package coderd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/updatecheck"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateCheck_NewVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		resp github.RepositoryRelease
		want codersdk.UpdateCheckResponse
	}{
		{
			name: "New version",
			resp: github.RepositoryRelease{
				TagName: github.String("v99.999.999"),
				HTMLURL: github.String("https://someurl.com"),
			},
			want: codersdk.UpdateCheckResponse{
				Current: false,
				Version: "v99.999.999",
				URL:     "https://someurl.com",
			},
		},
		{
			name: "Same version",
			resp: github.RepositoryRelease{
				TagName: github.String(buildinfo.Version()),
				HTMLURL: github.String("https://someurl.com"),
			},
			want: codersdk.UpdateCheckResponse{
				Current: true,
				Version: buildinfo.Version(),
				URL:     "https://someurl.com",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				b, err := json.Marshal(tt.resp)
				assert.NoError(t, err)
				w.Write(b)
			}))
			defer srv.Close()

			client := coderdtest.New(t, &coderdtest.Options{
				UpdateCheckOptions: &updatecheck.Options{
					URL: srv.URL,
				},
			})

			ctx := testutil.Context(t, testutil.WaitLong)

			got, err := client.UpdateCheck(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
