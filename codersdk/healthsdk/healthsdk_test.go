package healthsdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

func TestSummarize(t *testing.T) {
	t.Parallel()

	t.Run("HealthcheckReport", func(t *testing.T) {
		unhealthy := healthsdk.BaseReport{
			Error:    ptr.Ref("test error"),
			Warnings: []health.Message{{Code: "TEST", Message: "testing"}},
		}
		hr := healthsdk.HealthcheckReport{
			AccessURL: healthsdk.AccessURLReport{
				BaseReport: unhealthy,
			},
			Database: healthsdk.DatabaseReport{
				BaseReport: unhealthy,
			},
			DERP: healthsdk.DERPHealthReport{
				BaseReport: unhealthy,
			},
			ProvisionerDaemons: healthsdk.ProvisionerDaemonsReport{
				BaseReport: unhealthy,
			},
			Websocket: healthsdk.WebsocketReport{
				BaseReport: unhealthy,
			},
			WorkspaceProxy: healthsdk.WorkspaceProxyReport{
				BaseReport: unhealthy,
			},
		}
		expected := []string{
			"Access URL: Error: test error",
			"Access URL: Warn: TEST: testing",
			"Database: Error: test error",
			"Database: Warn: TEST: testing",
			"DERP: Error: test error",
			"DERP: Warn: TEST: testing",
			"Provisioner Daemons: Error: test error",
			"Provisioner Daemons: Warn: TEST: testing",
			"Websocket: Error: test error",
			"Websocket: Warn: TEST: testing",
			"Workspace Proxies: Error: test error",
			"Workspace Proxies: Warn: TEST: testing",
		}
		actual := hr.Summarize()
		assert.Equal(t, expected, actual)
	})

	for _, tt := range []struct {
		name     string
		br       healthsdk.BaseReport
		pfx      string
		expected []string
	}{
		{
			name:     "empty",
			br:       healthsdk.BaseReport{},
			pfx:      "",
			expected: []string{},
		},
		{
			name: "no prefix",
			br: healthsdk.BaseReport{
				Error: ptr.Ref("testing"),
				Warnings: []health.Message{
					{
						Code:    "TEST01",
						Message: "testing one",
					},
					{
						Code:    "TEST02",
						Message: "testing two",
					},
				},
			},
			pfx: "",
			expected: []string{
				"Error: testing",
				"Warn: TEST01: testing one",
				"Warn: TEST02: testing two",
			},
		},
		{
			name: "prefix",
			br: healthsdk.BaseReport{
				Error: ptr.Ref("testing"),
				Warnings: []health.Message{
					{
						Code:    "TEST01",
						Message: "testing one",
					},
					{
						Code:    "TEST02",
						Message: "testing two",
					},
				},
			},
			pfx: "TEST:",
			expected: []string{
				"TEST: Error: testing",
				"TEST: Warn: TEST01: testing one",
				"TEST: Warn: TEST02: testing two",
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := tt.br.Summarize(tt.pfx)
			if len(tt.expected) == 0 {
				assert.Empty(t, actual)
				return
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
