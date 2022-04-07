package session_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/access/session"
)

func TestSystemActor(t *testing.T) {
	t.Parallel()

	require.Equal(t, session.ActorTypeSystem, session.System.Type())
	require.Equal(t, session.SystemUserID, session.System.ID())
	require.Equal(t, session.SystemUserID, session.System.Name())
	session.System.System()
}
