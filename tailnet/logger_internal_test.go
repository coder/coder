package tailnet

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet/proto"
)

func TestBufferLogSink(t *testing.T) {
	t.Parallel()

	t.Run("NoIP", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		sink, err := newBufferLogSink()
		require.NoError(t, err)
		logger := slog.Make(sink).Leveled(slog.LevelDebug)

		logger.Debug(ctx, "line1")
		logger.Debug(ctx, "line2 fe80")
		logger.Debug(ctx, "line3 xxxx::x")

		logs, hashes := sink.getLogs()
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
				sink, err := newBufferLogSink()
				require.NoError(t, err)
				logger := slog.Make(sink).Leveled(slog.LevelDebug)

				ipWithPort := c.ip + ":8080"
				if c.expectedVersion == 6 {
					ipWithPort = fmt.Sprintf("[%s]:8080", c.ip)
				}

				logger.Debug(ctx, "line1", slog.F("ip", c.ip))
				logger.Debug(ctx, fmt.Sprintf("line2: %s/24", c.ip))
				logger.Debug(ctx, fmt.Sprintf("line3: %s foo (%s)", ipWithPort, c.ip))

				logs, hashes := sink.getLogs()
				require.Len(t, logs, 3)
				require.Len(t, hashes, 1)
				for _, log := range logs {
					t.Log(log)
				}

				// This only runs once since we only processed a single IP.
				for expectedHash, ipFields := range hashes {
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
}
