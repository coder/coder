package coderd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostCSPViolations(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)

	// oversizedReportBody builds a JSON body well over the 64KB limit
	// enforced by coderd.cspReportMaxBytes, mirroring the Cure53 PoC of
	// posting oversized bodies to force unbounded heap allocation.
	oversizedReportBody := func() []byte {
		padding := strings.Repeat("a", 128*1024)
		return []byte(`{"csp-report":{"padding":"` + padding + `"}}`)
	}

	tests := []struct {
		name           string
		body           any
		expectedStatus int
	}{
		{
			name: "OK",
			body: map[string]any{
				"csp-report": map[string]any{
					"document-uri":       "https://example.com",
					"violated-directive": "script-src",
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "OversizedBody",
			body:           oversizedReportBody(),
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			res, err := client.Request(ctx, http.MethodPost, "/api/v2/csp/reports", tt.body)
			require.NoError(t, err)
			defer res.Body.Close()

			if tt.expectedStatus != http.StatusOK {
				apiErr := codersdk.ReadBodyAsError(res)
				var sdkErr *codersdk.Error
				require.ErrorAs(t, apiErr, &sdkErr)
				require.Equal(t, tt.expectedStatus, sdkErr.StatusCode())
				return
			}
			require.Equal(t, tt.expectedStatus, res.StatusCode)
		})
	}
}
