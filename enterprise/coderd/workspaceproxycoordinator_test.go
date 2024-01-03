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
	"tailscale.com/types/key"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// workspaceProxyCoordinate and agentIsLegacy are both tested by wsproxy tests.

func Test_agentIsLegacy(t *testing.T) {
	t.Parallel()

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
		require.NoError(t, ma.UpdateSelf(&agpl.Node{
			ID:            55,
			AsOf:          time.Unix(1689653252, 0),
			Key:           key.NewNode().Public(),
			DiscoKey:      key.NewDisco().Public(),
			PreferredDERP: 0,
			DERPLatency: map[string]float64{
				"0": 1.0,
			},
			DERPForcedWebsocket: map[int]string{},
			Addresses:           []netip.Prefix{netip.PrefixFrom(codersdk.WorkspaceAgentIP, 128)},
			AllowedIPs:          []netip.Prefix{netip.PrefixFrom(codersdk.WorkspaceAgentIP, 128)},
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
		require.NoError(t, ma.UpdateSelf(&agpl.Node{
			ID:            55,
			AsOf:          time.Unix(1689653252, 0),
			Key:           key.NewNode().Public(),
			DiscoKey:      key.NewDisco().Public(),
			PreferredDERP: 0,
			DERPLatency: map[string]float64{
				"0": 1.0,
			},
			DERPForcedWebsocket: map[int]string{},
			Addresses:           []netip.Prefix{netip.PrefixFrom(agpl.IPFromUUID(nodeID), 128)},
			AllowedIPs:          []netip.Prefix{netip.PrefixFrom(agpl.IPFromUUID(nodeID), 128)},
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
