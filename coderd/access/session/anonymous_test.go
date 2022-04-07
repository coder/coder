package session_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/access/session"
)

func TestAnonymousActor(t *testing.T) {
	t.Parallel()

	require.Equal(t, session.ActorTypeAnonymous, session.Anon.Type())
	require.Equal(t, session.AnonymousUserID, session.Anon.ID())
	require.Equal(t, session.AnonymousUserID, session.Anon.Name())
	session.Anon.Anonymous()
}
