//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/testutil"
)

const procNetTCPHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// procNetRow formats one socket row of /proc/net/tcp or /proc/net/tcp6. The
// local address is in the kernel's hex form.
func procNetRow(addr string, port uint16, state string, uid int) string {
	return fmt.Sprintf("   0: %s:%04X 00000000:0000 %s 00000000:00000000 00:00000000 00000000 %5d        0 12345 1 0000000000000000 100 0 0 10 0\n",
		addr, port, state, uid)
}

const (
	testPort = 3000 // 0x0BB8

	ipv4Lo     = "0100007F"
	ipv4Any    = "00000000"
	ipv4Other  = "0501A8C0" // 192.168.1.5
	ipv6Any    = "00000000000000000000000000000000"
	ipv6Mapped = "0000000000000000FFFF00000100007F" // ::ffff:127.0.0.1
	ipv6Lo     = "00000000000000000000000001000000" // ::1
)

func TestLoopbackListeners(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want []loopbackListener
	}{
		{name: "Empty"},
		{name: "HeaderOnly", text: procNetTCPHeader},
		{
			name: "LoopbackOwnUID",
			text: procNetTCPHeader + procNetRow(ipv4Lo, testPort, "0A", 1000),
			want: []loopbackListener{{uid: 1000, ipv4: true}},
		},
		{
			name: "WildcardForeignUID",
			text: procNetTCPHeader + procNetRow(ipv4Any, testPort, "0A", 1001),
			want: []loopbackListener{{uid: 1001, ipv4: true}},
		},
		{
			name: "IPv6DualStackWildcard",
			text: procNetTCPHeader + procNetRow(ipv6Any, testPort, "0A", 1002),
			want: []loopbackListener{{uid: 1002, anyIPv6: true}},
		},
		{
			name: "IPv6MappedLoopback",
			text: procNetTCPHeader + procNetRow(ipv6Mapped, testPort, "0A", 1003),
			want: []loopbackListener{{uid: 1003}},
		},
		{
			name: "EveryMatchingRow",
			text: procNetTCPHeader +
				procNetRow(ipv4Lo, testPort, "0A", 1000) +
				procNetRow(ipv4Any, testPort, "0A", 1001),
			want: []loopbackListener{{uid: 1000, ipv4: true}, {uid: 1001, ipv4: true}},
		},
		{
			name: "OtherAddresses",
			text: procNetTCPHeader +
				procNetRow(ipv4Other, testPort, "0A", 1001) +
				procNetRow(ipv6Lo, testPort, "0A", 1001),
		},
		{
			name: "OtherPorts",
			text: procNetTCPHeader +
				procNetRow(ipv4Lo, testPort+1, "0A", 1001) +
				procNetRow(ipv4Any, 22, "0A", 1001),
		},
		{
			name: "NotListening",
			text: procNetTCPHeader +
				procNetRow(ipv4Lo, testPort, "01", 1001) + // ESTABLISHED
				procNetRow(ipv4Any, testPort, "06", 1001), // TIME_WAIT
		},
		{
			name: "MatchAmongOthers",
			text: procNetTCPHeader +
				procNetRow(ipv4Lo, testPort, "01", 1001) +
				procNetRow(ipv4Lo, 8080, "0A", 1001) +
				procNetRow(ipv4Any, testPort, "0A", 1000),
			want: []loopbackListener{{uid: 1000, ipv4: true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, loopbackListeners(tc.text, testPort))
		})
	}
}

func TestTableListenerUIDs(t *testing.T) {
	t.Parallel()

	// checkAPIListener compares with the uid of this process.
	self := os.Getuid()
	foreign := self + 1
	listen := func(addr string, uid int) string {
		return procNetRow(addr, testPort, "0A", uid)
	}
	cases := []struct {
		name string
		tcp  string
		tcp6 string
		want []int
	}{
		{name: "NoListener"},
		{
			name: "OwnIPv4",
			tcp:  procNetTCPHeader + listen(ipv4Any, self),
			want: []int{self},
		},
		{
			// Linux refuses a dual-stack socket next to an IPv4 listener, so
			// the foreign socket on :: is IPv6-only and gets no request.
			name: "OwnIPv4AndForeignIPv6Wildcard",
			tcp:  procNetTCPHeader + listen(ipv4Any, self),
			tcp6: procNetTCPHeader + listen(ipv6Any, foreign),
			want: []int{self},
		},
		{
			name: "OwnLoopbackAndForeignIPv6Wildcard",
			tcp:  procNetTCPHeader + listen(ipv4Lo, self),
			tcp6: procNetTCPHeader + listen(ipv6Any, foreign),
			want: []int{self},
		},
		{
			name: "ForeignIPv6WildcardAlone",
			tcp6: procNetTCPHeader + listen(ipv6Any, foreign),
			want: []int{foreign},
		},
		{
			name: "OwnIPv4AndForeignIPv4Wildcard",
			tcp:  procNetTCPHeader + listen(ipv4Lo, self) + listen(ipv4Any, foreign),
			want: []int{self, foreign},
		},
		{
			name: "OwnIPv4AndForeignMappedLoopback",
			tcp:  procNetTCPHeader + listen(ipv4Any, self),
			tcp6: procNetTCPHeader + listen(ipv6Mapped, foreign),
			want: []int{self, foreign},
		},
		{
			name: "ForeignIPv4AndForeignIPv6Wildcard",
			tcp:  procNetTCPHeader + listen(ipv4Lo, foreign),
			tcp6: procNetTCPHeader + listen(ipv6Any, foreign),
			want: []int{foreign, foreign},
		},
		{
			// An IPv4 socket of another user does not make the socket on ::
			// harmless, so both still count.
			name: "ForeignIPv4AndOwnIPv6Wildcard",
			tcp:  procNetTCPHeader + listen(ipv4Lo, foreign),
			tcp6: procNetTCPHeader + listen(ipv6Any, self),
			want: []int{foreign, self},
		},
		{
			name: "OwnIPv6WildcardAlone",
			tcp6: procNetTCPHeader + listen(ipv6Any, self),
			want: []int{self},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uids := tableListenerUIDs(tc.tcp, tc.tcp6, testPort, self)
			assert.Equal(t, tc.want, uids)

			// A foreign uid in the result makes the check refuse.
			logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))
			err := checkAPIListener(testutil.Context(t, testutil.WaitShort), logger, testPort,
				func(int64) ([]int, error) { return uids, nil })
			if slices.Contains(uids, foreign) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("belongs to uid %d", foreign))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcListenerUIDs reads the real socket tables, so the address byte
// order of the parser is checked against the kernel's output.
func TestProcListenerUIDs(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/net/tcp")
	}

	for _, network := range []struct{ name, network, addr string }{
		{"Loopback", "tcp4", "127.0.0.1:0"},
		{"Wildcard", "tcp", ":0"},
	} {
		t.Run(network.name, func(t *testing.T) {
			t.Parallel()
			l, err := net.Listen(network.network, network.addr)
			require.NoError(t, err)
			t.Cleanup(func() { _ = l.Close() })

			uids, err := procListenerUIDs(int64(l.Addr().(*net.TCPAddr).Port))
			require.NoError(t, err)
			assert.Contains(t, uids, os.Getuid())
		})
	}
}

func TestCheckAPIListener(t *testing.T) {
	t.Parallel()

	const port = 3000
	self := os.Getuid()
	foreign := self + 1
	lookup := func(uids []int, err error) listenerLookup {
		return func(got int64) ([]int, error) {
			assert.EqualValues(t, port, got)
			return uids, err
		}
	}
	cases := []struct {
		name     string
		lookup   listenerLookup
		wantErr  string
		wantWarn string
	}{
		{name: "OwnUID", lookup: lookup([]int{self}, nil)},
		{
			name:    "ForeignUID",
			lookup:  lookup([]int{foreign}, nil),
			wantErr: fmt.Sprintf("API port %d belongs to uid %d", port, foreign),
		},
		{
			name:    "OwnAndForeignUID",
			lookup:  lookup([]int{self, foreign}, nil),
			wantErr: fmt.Sprintf("API port %d belongs to uid %d", port, foreign),
		},
		{
			name:     "NoListener",
			lookup:   lookup(nil, nil),
			wantWarn: "found no listener",
		},
		{
			name:     "ReadError",
			lookup:   lookup(nil, xerrors.New("permission denied")),
			wantWarn: "cannot check who owns",
		},
		{name: "Unsupported", lookup: lookup(nil, errListenerCheckUnsupported)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := slog.Make(sloghuman.Sink(&buf))
			err := checkAPIListener(testutil.Context(t, testutil.WaitShort), logger, port, tc.lookup)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantWarn == "" {
				assert.NotContains(t, buf.String(), "WARN")
			} else {
				assert.Contains(t, buf.String(), tc.wantWarn)
			}
		})
	}
}

func TestSetupFirstUserListenerOwner(t *testing.T) {
	t.Parallel()

	// A server that records every request. The config directory holds a
	// saved session token, which setup sends first when it reuses data.
	newSetup := func(t *testing.T) (*devConfig, *atomic.Int32, string) {
		t.Helper()
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		configDir := t.TempDir()
		require.NoError(t, config.Root(configDir).Session().Write("test-saved-session-token"))
		cfg := &devConfig{
			apiPort:   int64(srv.Listener.Addr().(*net.TCPAddr).Port),
			configDir: configDir,
			password:  "test-admin-password",
		}
		return cfg, &requests, srv.URL
	}
	logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))

	t.Run("ForeignUID", func(t *testing.T) {
		t.Parallel()
		cfg, requests, apiURL := newSetup(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := setupFirstUser(ctx, logger, cfg, apiURL, func(int64) ([]int, error) {
			return []int{os.Getuid() + 1}, nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("API port %d belongs to uid %d", cfg.apiPort, os.Getuid()+1))
		assert.EqualValues(t, 0, requests.Load(), "setup must send nothing to a foreign listener")
	})

	t.Run("OwnUID", func(t *testing.T) {
		t.Parallel()
		cfg, requests, apiURL := newSetup(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The server answers 404, so setup fails after its first request.
		// The test only needs to see that setup runs.
		_, err := setupFirstUser(ctx, logger, cfg, apiURL, func(int64) ([]int, error) {
			return []int{os.Getuid()}, nil
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "belongs to uid")
		assert.Positive(t, requests.Load())
	})
}
