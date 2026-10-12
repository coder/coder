package chatd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatAutomationInterval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{time.Minute, "1 minute"},
		{5 * time.Minute, "5 minutes"},
		// A minimum that is not whole minutes must not round down to the
		// interval it is compared with.
		{2*time.Minute + 30*time.Second, "2 minutes 30 seconds"},
		{61 * time.Second, "1 minute 1 second"},
		{90 * time.Minute, "90 minutes"},
		{time.Hour, "1 hour"},
		{24 * time.Hour, "24 hours"},
	} {
		require.Equal(t, tc.want, formatAutomationInterval(tc.in), tc.in.String())
	}
}
