package tailnet

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

func TestTelemetryStore(t *testing.T) {
	t.Parallel()

	t.Run("NoIP", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		sink, err := newTelemetryStore()
		require.NoError(t, err)
		logger := slog.Make(sink).Leveled(slog.LevelDebug)

		logger.Debug(ctx, "line1")
		logger.Debug(ctx, "line2 fe80")
		logger.Debug(ctx, "line3 xxxx::x")

		logs, hashes, _, _ := sink.getStore()
		require.Len(t, logs, 3)
		require.Len(t, hashes, 0)
		require.Contains(t, logs[0], "line1")
		require.Contains(t, logs[1], "line2 fe80")
		require.Contains(t, logs[2], "line3 xxxx::x")
	})

	t.Run("OneOrMoreIPs", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name            string
			ip              string
			expectedVersion int32
			expectedClass   proto.IPFields_IPClass
		}{
			{
				name:            "IPv4/Public",
				ip:              "142.250.71.78",
				expectedVersion: 4,
				expectedClass:   proto.IPFields_PUBLIC,
			},
			{
				name:            "IPv4/Private",
				ip:              "192.168.0.1",
				expectedVersion: 4,
				expectedClass:   proto.IPFields_PRIVATE,
			},
			{
				name:            "IPv4/LinkLocal",
				ip:              "169.254.1.1",
				expectedVersion: 4,
				expectedClass:   proto.IPFields_LINK_LOCAL,
			},
			{
				name:            "IPv4/Loopback",
				ip:              "127.0.0.1",
				expectedVersion: 4,
				expectedClass:   proto.IPFields_LOOPBACK,
			},
			{
				name:            "IPv6/Public",
				ip:              "2404:6800:4006:812::200e",
				expectedVersion: 6,
				expectedClass:   proto.IPFields_PUBLIC,
			},
			{
				name:            "IPv6/Private",
				ip:              "fd12:3456:789a:1::1",
				expectedVersion: 6,
				expectedClass:   proto.IPFields_PRIVATE,
			},
			{
				name:            "IPv6/LinkLocal",
				ip:              "fe80::1",
				expectedVersion: 6,
				expectedClass:   proto.IPFields_LINK_LOCAL,
			},
			{
				name:            "IPv6/Loopback",
				ip:              "::1",
				expectedVersion: 6,
				expectedClass:   proto.IPFields_LOOPBACK,
			},
			{
				name:            "IPv6/IPv4Mapped",
				ip:              "::ffff:1.2.3.4",
				expectedVersion: 6,
				expectedClass:   proto.IPFields_PUBLIC,
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				sink, err := newTelemetryStore()
				require.NoError(t, err)
				logger := slog.Make(sink).Leveled(slog.LevelDebug)

				ipWithPort := c.ip + ":8080"
				if c.expectedVersion == 6 {
					ipWithPort = fmt.Sprintf("[%s]:8080", c.ip)
				}

				logger.Debug(ctx, "line1", slog.F("ip", c.ip))
				logger.Debug(ctx, fmt.Sprintf("line2: %s/24", c.ip))
				logger.Debug(ctx, fmt.Sprintf("line3: %s foo (%s)", ipWithPort, c.ip))

				logs, ips, _, _ := sink.getStore()
				require.Len(t, logs, 3)
				require.Len(t, ips, 1)
				for _, log := range logs {
					t.Log(log)
				}

				// This only runs once since we only processed a single IP.
				for expectedHash, ipFields := range ips {
					hashedIPWithPort := expectedHash + ":8080"
					if c.expectedVersion == 6 {
						hashedIPWithPort = fmt.Sprintf("[%s]:8080", expectedHash)
					}

					require.Contains(t, logs[0], "line1")
					require.Contains(t, logs[0], "ip="+expectedHash)
					require.Contains(t, logs[1], fmt.Sprintf("line2: %s/24", expectedHash))
					require.Contains(t, logs[2], fmt.Sprintf("line3: %s foo (%s)", hashedIPWithPort, expectedHash))

					require.Equal(t, c.expectedVersion, ipFields.Version)
					require.Equal(t, c.expectedClass, ipFields.Class)
				}
			})
		}
	})

	t.Run("DerpMapClean", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		telemetry, err := newTelemetryStore()
		require.NoError(t, err)
		logger := slog.Make(telemetry).Leveled(slog.LevelDebug)

		derpMap := &tailcfg.DERPMap{
			Regions: make(map[int]*tailcfg.DERPRegion),
		}
		// Add a region and node that uses every single field.
		derpMap.Regions[999] = &tailcfg.DERPRegion{
			RegionID:      999,
			EmbeddedRelay: true,
			RegionCode:    "zzz",
			RegionName:    "Cool Region",
			Avoid:         true,

			Nodes: []*tailcfg.DERPNode{
				{
					Name:       "zzz1",
					RegionID:   999,
					HostName:   "coolderp.com",
					CertName:   "coolderpcert",
					IPv4:       "1.2.3.4",
					IPv6:       "2001:db8::1",
					STUNTestIP: "5.6.7.8",
				},
			},
		}
		telemetry.updateDerpMap(derpMap)

		logger.Debug(ctx, "line1 coolderp.com qwerty")
		logger.Debug(ctx, "line2 1.2.3.4 asdf")
		logger.Debug(ctx, "line3 2001:db8::1 foo")

		logs, ips, dm, _ := telemetry.getStore()
		require.Len(t, logs, 3)
		require.Len(t, ips, 3)
		require.Len(t, dm.Regions[999].Nodes, 1)
		node := dm.Regions[999].Nodes[0]
		require.NotContains(t, node.HostName, "coolderp.com")
		require.NotContains(t, node.IPv4, "1.2.3.4")
		require.NotContains(t, node.IPv6, "2001:db8::1")
		require.NotContains(t, node.STUNTestIP, "5.6.7.8")
		require.Contains(t, logs[0], node.HostName)
		require.Contains(t, ips, node.STUNTestIP)
		require.Contains(t, ips, node.IPv6)
		require.Contains(t, ips, node.IPv4)
	})
}
