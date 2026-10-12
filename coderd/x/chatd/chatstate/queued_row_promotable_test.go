package chatstate_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// TestQueuedRowPromotable pins the queue hygiene rule that promotion and
// chatd's admission prechecks share: only rows of an existing, enabled
// automation at the row's queue generation are delivered.
func TestQueuedRowPromotable(t *testing.T) {
	t.Parallel()
	enabled := database.ChatAutomation{ID: uuid.New(), Enabled: true, QueueGeneration: 3}
	disabled := database.ChatAutomation{ID: uuid.New(), Enabled: false, QueueGeneration: 3}
	automations := map[uuid.UUID]database.ChatAutomation{enabled.ID: enabled, disabled.ID: disabled}
	row := func(id uuid.UUID, generation sql.NullInt64) database.ChatQueuedMessage {
		return database.ChatQueuedMessage{AutomationID: uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}, QueueGeneration: generation}
	}
	gen := func(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: true} }

	for _, tc := range []struct {
		name string
		row  database.ChatQueuedMessage
		want bool
	}{
		{"HumanRow", row(uuid.Nil, sql.NullInt64{}), true},
		{"CurrentGeneration", row(enabled.ID, gen(3)), true},
		{"OlderGeneration", row(enabled.ID, gen(2)), false},
		{"MissingGeneration", row(enabled.ID, sql.NullInt64{}), false},
		{"DisabledAutomation", row(disabled.ID, gen(3)), false},
		{"DeletedAutomation", row(uuid.New(), gen(3)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, chatstate.QueuedRowPromotable(tc.row, automations))
		})
	}
}
