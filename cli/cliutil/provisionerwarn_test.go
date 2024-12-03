package cliutil_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
)

func TestWarnMatchedProvisioners(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		mp     *codersdk.MatchedProvisioners
		job    codersdk.ProvisionerJob
		expect string
	}{
		{
			name: "no_match",
			mp: &codersdk.MatchedProvisioners{
				Count:     0,
				Available: 0,
			},
			job: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
			expect: `there are no provisioners that accept the required tags`,
		},
		{
			name: "no_available",
			mp: &codersdk.MatchedProvisioners{
				Count:     1,
				Available: 0,
			},
			job: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
			expect: `Provisioners that accept the required tags have not responded for longer than expected`,
		},
		{
			name: "match",
			mp: &codersdk.MatchedProvisioners{
				Count:     1,
				Available: 1,
			},
			job: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobPending,
			},
		},
		{
			name: "not_pending",
			mp:   &codersdk.MatchedProvisioners{},
			job: codersdk.ProvisionerJob{
				Status: codersdk.ProvisionerJobRunning,
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var w strings.Builder
			cliutil.WarnMatchedProvisioners(&w, tt.mp, tt.job)
			if tt.expect != "" {
				require.Contains(t, w.String(), tt.expect)
			} else {
				require.Empty(t, w.String())
			}
		})
	}
}
