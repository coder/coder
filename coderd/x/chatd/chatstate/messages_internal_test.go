package chatstate

import (
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestTurnEnvironmentVariablesPersistence(t *testing.T) {
	t.Parallel()

	environmentVariables := pqtype.NullRawMessage{
		RawMessage: []byte(`{"SCANNER_TOKEN":"secret"}`),
		Valid:      true,
	}
	params := toInsertParams(uuid.New(), []Message{{ //nolint:gosec // Test-only placeholder value.
		Role:                 database.ChatMessageRoleUser,
		Content:              pqtype.NullRawMessage{RawMessage: []byte(`[]`), Valid: true},
		EnvironmentVariables: environmentVariables,
	}})
	require.Equal(t, string(environmentVariables.RawMessage), params.EnvironmentVariables[0])

	promoted := messageFromQueuedRow(database.ChatQueuedMessage{
		Content:              []byte(`[]`),
		EnvironmentVariables: environmentVariables,
	})
	require.Equal(t, environmentVariables, promoted.EnvironmentVariables)
}
