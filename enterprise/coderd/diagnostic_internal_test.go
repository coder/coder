package coderd

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestClassifySessionStatusUnexpectedDisconnect(t *testing.T) {
	t.Parallel()

	status := classifySessionStatus(
		"connection ended unexpectedly: session closed without explicit reason (exit code: 1)",
		true,
		false,
	)
	assert.Equal(t, codersdk.ConnectionStatusControlLost, status)
}

func TestClassifySessionStatusProcessExitMinusOne(t *testing.T) {
	t.Parallel()

	status := classifySessionStatus(
		"process exited with error status: -1",
		true,
		false,
	)
	assert.Equal(t, codersdk.ConnectionStatusControlLost, status)
}

func TestGenerateExplanationUnexpectedDisconnect(t *testing.T) {
	t.Parallel()

	explanation := generateExplanation(
		"connection ended unexpectedly: session closed without explicit reason (exit code: 1)",
		false,
	)
	assert.Equal(t, "Connection lost unexpectedly.", explanation)
}

func TestGenerateExplanationProcessExitMinusOne(t *testing.T) {
	t.Parallel()

	explanation := generateExplanation(
		"process exited with error status: -1",
		false,
	)
	assert.Equal(t, "Connection lost unexpectedly.", explanation)
}

func TestConvertSessionConnectionUnexpectedDisconnect(t *testing.T) {
	t.Parallel()

	conn := convertSessionConnection(database.ConnectionLog{
		ID:   uuid.New(),
		Type: database.ConnectionTypeSsh,
		DisconnectTime: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
		DisconnectReason: sql.NullString{
			String: "connection ended unexpectedly: session closed without explicit reason (exit code: 1)",
			Valid:  true,
		},
	})

	assert.Equal(t, codersdk.ConnectionStatusControlLost, conn.Status)
	assert.Equal(t, "Connection lost unexpectedly.", conn.Explanation)
}

func TestBuildTimelineIncludesDisconnectCodeForUnexpectedDisconnect(t *testing.T) {
	t.Parallel()

	now := time.Now()
	events := buildTimeline([]database.ConnectionLog{{
		ID:          uuid.New(),
		ConnectTime: now,
		Type:        database.ConnectionTypeSsh,
		Code: sql.NullInt32{
			Int32: 1,
			Valid: true,
		},
		DisconnectTime: sql.NullTime{
			Time:  now.Add(time.Minute),
			Valid: true,
		},
		DisconnectReason: sql.NullString{
			String: "connection ended unexpectedly: session closed without explicit reason (exit code: 1)",
			Valid:  true,
		},
	}})

	assert.Len(t, events, 2)

	closeEvent := events[1]
	assert.Equal(t, codersdk.DiagnosticTimelineEventConnectionClosed, closeEvent.Kind)
	assert.Equal(t, codersdk.ConnectionDiagnosticSeverityError, closeEvent.Severity)
	assert.Equal(t, int32(1), closeEvent.Metadata["disconnect_code"])
}
