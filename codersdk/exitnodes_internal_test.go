package codersdk

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestExitNodeReplicaPeerID(t *testing.T) {
	t.Parallel()

	exitNodeID := uuid.New()
	replicaID := uuid.New()
	peerID := ExitNodeReplicaPeerID(exitNodeID, replicaID)
	require.Equal(t, peerID, ExitNodeReplicaPeerID(exitNodeID, replicaID))
	require.NotEqual(t, replicaID, peerID)
	require.NotEqual(t, peerID, ExitNodeReplicaPeerID(uuid.New(), replicaID))
}
