package entity_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entity"
)

func TestType(t *testing.T) {
	t.Parallel()

	// The set is closed, so this doubles as the list of what is in it.
	for _, valid := range []entity.Type{
		entity.TypeAIAgent,
		entity.TypeWorkspaceAgent,
		entity.TypeUser,
	} {
		require.True(t, valid.Valid(), "%q should be a known type", valid)
	}

	for _, invalid := range []entity.Type{
		"",
		"sandbox",
		"agent",
		"AI_AGENT",
		"ai_agents",
	} {
		require.False(t, invalid.Valid(), "%q should not be a known type", invalid)
	}
}
