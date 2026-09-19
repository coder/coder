package exitnode_test

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/enterprise/exitnode/exitnodesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNewRequiresReplicaID(t *testing.T) {
	t.Parallel()
	_, err := exitnode.New(t.Context(), testutil.Logger(t), exitnode.Options{Client: exitnodesdk.New(&url.URL{Scheme: "http", Host: "localhost"}, "token"), ExitNodeID: uuid.New(), Policy: exitnode.PolicyFunc(func(exitnode.FlowInfo) exitnode.Decision { return exitnode.Decision{} })})
	require.ErrorContains(t, err, "replica id is required")
}
func TestTailnetAddrForReplicaID(t *testing.T) {
	t.Parallel()
	first := uuid.New()
	require.NotEqual(t, exitnode.TailnetAddrForID(first), exitnode.TailnetAddrForID(uuid.New()))
	require.Equal(t, exitnode.TailnetAddrForID(first), exitnode.TailnetAddrForID(first))
}
