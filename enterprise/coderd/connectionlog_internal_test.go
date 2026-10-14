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
			isSSH := method == database.ConnectionLogMethodSSH || method == database.ConnectionLogMethodReconnectingPTY
			require.Equal(t, isSSH, log.SSHInfo != nil)
			require.Equal(t, !isSSH, log.WebInfo != nil)
		})
	}
}

func TestConvertConnectionLogDuration(t *testing.T) {
	t.Parallel()

	const (
		ssh = database.ConnectionLogMethodSSH
		pty = database.ConnectionLogMethodReconnectingPTY

		connect    = database.ConnectionLogConnectTimeSourceConnect
		disconnect = database.ConnectionLogConnectTimeSourceDisconnect
		unknown    = database.ConnectionLogConnectTimeSourceUnknown

		recorded       = codersdk.ConnectionLogDurationRecorded
		unverified     = codersdk.ConnectionLogDurationUnverified
		noDisconnect   = codersdk.ConnectionLogDurationNoDisconnect
		missingConnect = codersdk.ConnectionLogDurationMissingConnect
		invalid        = codersdk.ConnectionLogDurationInvalid
	)
	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	end := start.Add(90*time.Second + 250*time.Millisecond)
	longStart, longEnd := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC)
	closed := func(at time.Time) sql.NullTime { return sql.NullTime{Time: at, Valid: true} }

	for _, tt := range []struct {
		name    string
		method  database.ConnectionLogMethod
		source  database.ConnectionLogConnectTimeSource
		start   time.Time
		end     sql.NullTime
		status  codersdk.ConnectionLogDurationStatus
		seconds *float64
	}{
		{"Recorded", ssh, connect, start, closed(end), recorded, new(90.25)},
		{"RecordedReconnectingPTY", pty, connect, start, closed(end), recorded, new(90.25)},
		{"ZeroRecorded", ssh, connect, start, closed(start), recorded, new(0.0)},
		{"FractionalSecond", ssh, connect, start.Add(900 * time.Millisecond), closed(start.Add(1100 * time.Millisecond)), recorded, new(0.2)},
		{"LongStoredInterval", ssh, unknown, longStart, closed(longEnd), unverified, new(12622780800.0)},
		{"Historical", ssh, unknown, start, closed(end), unverified, new(90.25)},
		{"HistoricalZero", ssh, unknown, start, closed(start), unverified, new(0.0)},
		{"NoDisconnect", ssh, connect, start, sql.NullTime{}, noDisconnect, nil},
		{"HistoricalNoDisconnect", ssh, unknown, start, sql.NullTime{}, noDisconnect, nil},
		{"MissingConnect", ssh, disconnect, start, closed(end), missingConnect, nil},
		{"Reversed", ssh, connect, end, closed(start), invalid, nil},
		{"UnknownStart", ssh, unknown, time.Time{}, closed(end), invalid, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := convertConnectionLog(database.GetConnectionLogsOffsetRow{
				ConnectionLog: database.ConnectionLog{
					ConnectionMethod:  tt.method,
					ConnectTime:       tt.start,
					DisconnectTime:    tt.end,
					ConnectTimeSource: tt.source,
				},
			})
			require.Equal(t, tt.status, log.SSHInfo.DurationStatus)
			if tt.seconds == nil {
				require.Nil(t, log.SSHInfo.DurationSeconds)
			} else {
				require.NotNil(t, log.SSHInfo.DurationSeconds)
				require.InDelta(t, *tt.seconds, *log.SSHInfo.DurationSeconds, 1e-9)
			}
		})
	}
}
