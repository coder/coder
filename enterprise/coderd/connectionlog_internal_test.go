package coderd

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
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

func TestConvertConnectionLogDuration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	end := start.Add(90*time.Second + 250*time.Millisecond)
	closed := func(at time.Time) sql.NullTime { return sql.NullTime{Time: at, Valid: true} }

	for _, tt := range []struct {
		name    string
		start   time.Time
		end     sql.NullTime
		status  codersdk.ConnectionLogDurationStatus
		seconds *float64
	}{
		{"Recorded", start, closed(end), codersdk.ConnectionLogDurationRecorded, new(90.25)},
		{"NoDisconnect", start, sql.NullTime{}, codersdk.ConnectionLogDurationNoDisconnect, nil},
		{"MissingConnect", end, closed(end), codersdk.ConnectionLogDurationMissingConnect, nil},
		{"Reversed", end, closed(start), codersdk.ConnectionLogDurationInvalid, nil},
		{"ZeroStart", time.Time{}, closed(end), codersdk.ConnectionLogDurationInvalid, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := convertConnectionLog(database.GetConnectionLogsOffsetRow{
				ConnectionLog: database.ConnectionLog{
					ConnectionMethod: database.ConnectionLogMethodSSH,
					ConnectTime:      tt.start,
					DisconnectTime:   tt.end,
				},
			})
			require.Equal(t, codersdk.ConnectionLogDuration{Status: tt.status, Seconds: tt.seconds}, log.SSHInfo.Duration)
		})
	}
}
