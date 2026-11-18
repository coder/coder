package healthcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/testutil"
)

func TestPubsub(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		dismissed bool
	}{
		{name: "OK"},
		{name: "Dismissed", dismissed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			var report healthcheck.PubsubReport
			report.Run(ctx, &healthcheck.PubsubReportOptions{
				Pubsub:    pubsub.NewInMemory(),
				Dismissed: tc.dismissed,
			})

			assert.Equal(t, health.SeverityOK, report.Severity)
			assert.Equal(t, pubsub.BackendMemory, report.Backend)
			assert.Equal(t, tc.dismissed, report.Dismissed)
		})
	}
}
