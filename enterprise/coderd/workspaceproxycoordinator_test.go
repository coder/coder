package coderd_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/types/key"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

// workspaceProxyCoordinate and agentIsLegacy are both tested by wsproxy tests.

func Test_agentIsLegacy(t *testing.T) {
	t.Parallel()
	nodeKey := key.NewNode().Public()
	discoKey := key.NewDisco().Public()
	nkBin, err := nodeKey.MarshalBinary()
	require.NoError(t, err)
	dkBin, err := discoKey.MarshalText()
	require.NoError(t, err)

	t.Run("Legacy", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			db, pubsub  = dbtestutil.NewDB(t)
			logger      = slogtest.Make(t, nil)
			coordinator = agpl.NewCoordinator(logger)
			client, _   = coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Database:    db,
					Pubsub:      pubsub,
					Coordinator: coordinator,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspaceProxy: 1,
					},
				},
			})
		)
		defer cancel()

		nodeID := uuid.New()
		ma := coordinator.ServeMultiAgent(nodeID)
		defer ma.Close()
		require.NoError(t, ma.UpdateSelf(&proto.Node{
			Id:            55,
			AsOf:          timestamppb.New(time.Unix(1689653252, 0)),
			Key:           nkBin,
			Disco:         string(dkBin),
			PreferredDerp: 0,
			DerpLatency: map[string]float64{
				"0": 1.0,
			},
			DerpForcedWebsocket: map[int32]string{},
			Addresses:           []string{codersdk.WorkspaceAgentIP.String() + "/128"},
			AllowedIps:          []string{codersdk.WorkspaceAgentIP.String() + "/128"},
			Endpoints:           []string{"192.168.1.1:18842"},
		}))
		require.Eventually(t, func() bool {
			return coordinator.Node(nodeID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		legacyRes, err := proxyClient.AgentIsLegacy(ctx, nodeID)
		require.NoError(t, err)

		assert.True(t, legacyRes.Found)
		assert.True(t, legacyRes.Legacy)
	})

	t.Run("NotLegacy", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
			db, pubsub  = dbtestutil.NewDB(t)
			logger      = slogtest.Make(t, nil)
			coordinator = agpl.NewCoordinator(logger)
			client, _   = coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Database:    db,
					Pubsub:      pubsub,
					Coordinator: coordinator,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureWorkspaceProxy: 1,
					},
				},
			})
		)
		defer cancel()

		nodeID := uuid.New()
		ma := coordinator.ServeMultiAgent(nodeID)
		defer ma.Close()
		require.NoError(t, ma.UpdateSelf(&proto.Node{
			Id:            55,
			AsOf:          timestamppb.New(time.Unix(1689653252, 0)),
			Key:           nkBin,
			Disco:         string(dkBin),
			PreferredDerp: 0,
			DerpLatency: map[string]float64{
				"0": 1.0,
			},
			DerpForcedWebsocket: map[int32]string{},
			Addresses:           []string{netip.PrefixFrom(agpl.IPFromUUID(nodeID), 128).String()},
			AllowedIps:          []string{netip.PrefixFrom(agpl.IPFromUUID(nodeID), 128).String()},
			Endpoints:           []string{"192.168.1.1:18842"},
		}))
		require.Eventually(t, func() bool {
			return coordinator.Node(nodeID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		proxyRes, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
			Name: namesgenerator.GetRandomName(1),
			Icon: "/emojis/flag.png",
		})
		require.NoError(t, err)

		proxyClient := wsproxysdk.New(client.URL)
		proxyClient.SetSessionToken(proxyRes.ProxyToken)

		legacyRes, err := proxyClient.AgentIsLegacy(ctx, nodeID)
		require.NoError(t, err)

		assert.True(t, legacyRes.Found)
		assert.False(t, legacyRes.Legacy)
	})
}
