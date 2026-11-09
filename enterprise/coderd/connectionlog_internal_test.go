package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// convertConnectionLog panics on a method it does not handle, so every method
// must convert with exactly one of WebInfo and SSHInfo.
func TestConvertConnectionLogHandlesEveryMethod(t *testing.T) {
	t.Parallel()

	for _, method := range database.AllConnectionLogMethodValues() {
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()

			log := convertConnectionLog(database.GetConnectionLogsOffsetRow{
				ConnectionLog: database.ConnectionLog{ConnectionMethod: method},
			})
			require.NotEqual(t, log.WebInfo == nil, log.SSHInfo == nil)
		})
	}
}
