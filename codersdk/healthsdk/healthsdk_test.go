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
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
			"Database: Error: test error",
			"Database: Warn: TEST: testing",
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
			"DERP: Error: test error",
			"DERP: Warn: TEST: testing",
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
			"Provisioner Daemons: Error: test error",
			"Provisioner Daemons: Warn: TEST: testing",
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
			"Websocket: Error: test error",
			"Websocket: Warn: TEST: testing",
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
			"Workspace Proxies: Error: test error",
			"Workspace Proxies: Warn: TEST: testing",
			"See: https://coder.com/docs/admin/monitoring/health-check#test",
		}
		actual := hr.Summarize("")
		assert.Equal(t, expected, actual)
	})

	for _, tt := range []struct {
		name     string
		br       healthsdk.BaseReport
		pfx      string
		docsURL  string
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
				"See: https://coder.com/docs/admin/monitoring/health-check#test01",
				"Warn: TEST02: testing two",
				"See: https://coder.com/docs/admin/monitoring/health-check#test02",
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
				"See: https://coder.com/docs/admin/monitoring/health-check#test01",
				"TEST: Warn: TEST02: testing two",
				"See: https://coder.com/docs/admin/monitoring/health-check#test02",
			},
		},
		{
			name: "custom docs url",
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
			docsURL: "https://my.coder.internal/docs",
			expected: []string{
				"Error: testing",
				"Warn: TEST01: testing one",
				"See: https://my.coder.internal/docs/admin/monitoring/health-check#test01",
				"Warn: TEST02: testing two",
				"See: https://my.coder.internal/docs/admin/monitoring/health-check#test02",
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := tt.br.Summarize(tt.pfx, tt.docsURL)
			if len(tt.expected) == 0 {
				assert.Empty(t, actual)
				return
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}
