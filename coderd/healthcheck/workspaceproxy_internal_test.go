package healthcheck

import (
	"fmt"
	"testing"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/xerrors"
)

func Test_WorkspaceProxyReport_appendErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		expected string
		prevErr  string
		errs     []error
	}{
		{
			name: "nil",
			errs: nil,
		},
		{
			name:     "one error",
			expected: assert.AnError.Error(),
			errs:     []error{assert.AnError},
		},
		{
			name:     "one error, one prev",
			prevErr:  "previous error",
			expected: "previous error\n" + assert.AnError.Error(),
			errs:     []error{assert.AnError},
		},
		{
			name:     "two errors",
			expected: assert.AnError.Error() + "\nanother error",
			errs:     []error{assert.AnError, xerrors.Errorf("another error")},
		},
		{
			name:     "two errors, one prev",
			prevErr:  "previous error",
			expected: "previous error\n" + assert.AnError.Error() + "\nanother error",
			errs:     []error{assert.AnError, xerrors.Errorf("another error")},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rpt WorkspaceProxyReport
			if tt.prevErr != "" {
				rpt.Error = ptr.Ref(tt.prevErr)
			}
			rpt.appendError(tt.errs...)
			if tt.expected == "" {
				require.Nil(t, rpt.Error)
			} else {
				require.NotNil(t, rpt.Error)
				require.Equal(t, tt.expected, *rpt.Error)
			}
		})
	}
}

func Test_calculateSeverity(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		total    int
		healthy  int
		expected health.Severity
	}{
		{0, 0, health.SeverityOK},
		{1, 1, health.SeverityOK},
		{1, 0, health.SeverityError},
		{2, 2, health.SeverityOK},
		{2, 1, health.SeverityWarning},
		{2, 0, health.SeverityError},
	} {
		tt := tt
		name := fmt.Sprintf("%d total, %d healthy -> %s", tt.total, tt.healthy, tt.expected)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			actual := calculateSeverity(tt.total, tt.healthy)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
