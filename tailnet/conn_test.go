package tailnet_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/envknob"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTailnet(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap := tailnettest.RunDERPAndSTUN(t)
	t.Run("InstantClose", func(t *testing.T) {
		t.Parallel()
		conn, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)
		err = conn.Close()
		require.NoError(t, err)
	})
	t.Run("Connect", func(t *testing.T) {
		t.Parallel()
		w1IP := tailnet.IP()
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:    logger.Named("w1"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:    logger.Named("w2"),
			DERPMap:   derpMap,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = w1.Close()
			_ = w2.Close()
		})
		w1.SetNodeCallback(func(node *tailnet.Node) {
			err := w2.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		w2.SetNodeCallback(func(node *tailnet.Node) {
			err := w1.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		require.True(t, w2.AwaitReachable(context.Background(), w1IP))
		conn := make(chan struct{}, 1)
		go func() {
			listener, err := w1.Listen("tcp", ":35565")
			assert.NoError(t, err)
			defer listener.Close()
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
			conn <- struct{}{}
		}()

		nc, err := w2.DialContextTCP(context.Background(), netip.AddrPortFrom(w1IP, 35565))
		require.NoError(t, err)
		_ = nc.Close()
		<-conn

		nodes := make(chan *tailnet.Node, 1)
		w2.SetNodeCallback(func(node *tailnet.Node) {
			select {
			case nodes <- node:
			default:
			}
		})
		node := <-nodes
		// Ensure this connected over DERP!
		require.Len(t, node.DERPForcedWebsocket, 0)

		w1.Close()
		w2.Close()
	})

	t.Run("ForcesWebSockets", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		w1IP := tailnet.IP()
		derpMap := tailnettest.RunDERPOnlyWebSockets(t)
		w1, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
			Logger:         logger.Named("w1"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)

		w2, err := tailnet.NewConn(&tailnet.Options{
			Addresses:      []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:         logger.Named("w2"),
			DERPMap:        derpMap,
			BlockEndpoints: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = w1.Close()
			_ = w2.Close()
		})
		w1.SetNodeCallback(func(node *tailnet.Node) {
			err := w2.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		w2.SetNodeCallback(func(node *tailnet.Node) {
			err := w1.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		})
		require.True(t, w2.AwaitReachable(ctx, w1IP))
		conn := make(chan struct{}, 1)
		go func() {
			listener, err := w1.Listen("tcp", ":35565")
			assert.NoError(t, err)
			defer listener.Close()
			nc, err := listener.Accept()
			if !assert.NoError(t, err) {
				return
			}
			_ = nc.Close()
			conn <- struct{}{}
		}()

		nc, err := w2.DialContextTCP(ctx, netip.AddrPortFrom(w1IP, 35565))
		require.NoError(t, err)
		_ = nc.Close()
		<-conn

		nodes := make(chan *tailnet.Node, 1)
		w2.SetNodeCallback(func(node *tailnet.Node) {
			select {
			case nodes <- node:
			default:
			}
		})
		node := <-nodes
		require.Len(t, node.DERPForcedWebsocket, 1)
		// Ensure the reason is valid!
		require.Equal(t, `GET failed with status code 400 (a proxy could be disallowing the use of 'Upgrade: derp'): Invalid "Upgrade" header: DERP`, node.DERPForcedWebsocket[derpMap.RegionIDs()[0]])

		w1.Close()
		w2.Close()
	})
}

// TestConn_PreferredDERP tests that we only trigger the NodeCallback when we have a preferred DERP server.
func TestConn_PreferredDERP(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	derpMap := tailnettest.RunDERPAndSTUN(t)
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:    logger.Named("w1"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)
	defer func() {
		err := conn.Close()
		require.NoError(t, err)
	}()
	// buffer channel so callback doesn't block
	nodes := make(chan *tailnet.Node, 50)
	conn.SetNodeCallback(func(node *tailnet.Node) {
		nodes <- node
	})
	select {
	case node := <-nodes:
		require.Equal(t, 1, node.PreferredDERP)
	case <-ctx.Done():
		t.Fatal("timed out waiting for node")
	}
}

//nolint:paralleltest
func TestTransmitHang_TCPSACK_Disabled(t *testing.T) {
	runTestTransmitHangMain(t)
}

//nolint:paralleltest
func TestTransmitHang_TCPSACK_Enabled(t *testing.T) {
	t.Setenv("TS_DEBUG_NETSTACK_ENABLE_TCPSACK", "true") // Restore after test.
	envknob.Setenv("TS_DEBUG_NETSTACK_ENABLE_TCPSACK", "true")
	runTestTransmitHangMain(t)
}

func runTestTransmitHangMain(t *testing.T) {
	curMaxProcs := runtime.GOMAXPROCS(0)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(curMaxProcs)
	})
	for _, env := range []string{"TS_DEBUG_DISCO", "TS_DEBUG_DERP", "TS_DEBUG_NETSTACK"} {
		t.Setenv(env, "true") // Restore after test.
		envknob.Setenv(env, "true")
	}

	for ok := true; ok; {
		for _, maxProcs := range []int{2, 3, curMaxProcs} {
			ok = t.Run(fmt.Sprintf("GOMAXPROCS=%d", maxProcs), func(t *testing.T) {
				t.Logf("GOMAXPROCS=%d", maxProcs)
				runtime.GOMAXPROCS(maxProcs)

				runTestTransmitHang(t, 20*time.Second)
			})
			if !ok {
				break
			}
		}
	}
}

func runTestTransmitHang(t *testing.T, timeout time.Duration) {
	// Not using t.TempDir() here so that we keep logs afterwards.
	captureDir, err := os.MkdirTemp("", "tailnet-test-")
	require.NoError(t, err)

	t.Cleanup(func() {
		// Only keep failed runs.
		if !t.Failed() {
			_ = os.RemoveAll(captureDir)
		}
	})

	testLog, err := os.Create(filepath.Join(captureDir, "test.log"))
	require.NoError(t, err)
	defer testLog.Close()
	recvCapture, err := os.Create(filepath.Join(captureDir, "recv.pcap"))
	require.NoError(t, err)
	defer recvCapture.Close()
	sendCapture, err := os.Create(filepath.Join(captureDir, "send.pcap"))
	require.NoError(t, err)
	defer sendCapture.Close()

	logger := slogtest.Make(t, nil).
		Leveled(slog.LevelDebug).
		AppendSinks(sloghuman.Sink(testLog))

	t.Logf("test log file: %v", testLog.Name())
	t.Logf("recv capture file: %v", recvCapture.Name())
	t.Logf("send capture file: %v", sendCapture.Name())

	logger.Info(context.Background(), "starting test", slog.F("GOMAXPROCS", runtime.GOMAXPROCS(0)), slog.F("timeout", timeout))

	ctx := context.Background()

	var derpMap *tailcfg.DERPMap
	pprof.Do(ctx, pprof.Labels("id", "tailnettest.derp-and-stun"), func(_ context.Context) {
		derpMap = tailnettest.RunDERPAndSTUN(t)
	})
	updateNodes := func(c *tailnet.Conn) func(*tailnet.Node) {
		return func(node *tailnet.Node) {
			err := c.UpdateNodes([]*tailnet.Node{node}, false)
			assert.NoError(t, err)
		}
	}

	recvIP := tailnet.IP()
	var recv *tailnet.Conn
	pprof.Do(ctx, pprof.Labels("id", "tailnet.recv"), func(_ context.Context) {
		recv, err = tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(recvIP, 128)},
			Logger:    logger.Named("recv"),
			DERPMap:   derpMap,
		})
	})
	require.NoError(t, err)
	defer recv.Close()
	recvCaptureStop := recv.Capture(recvCapture)
	defer recvCaptureStop()

	var send *tailnet.Conn
	pprof.Do(ctx, pprof.Labels("id", "tailnet.send"), func(_ context.Context) {
		send, err = tailnet.NewConn(&tailnet.Options{
			Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
			Logger:    logger.Named("send"),
			DERPMap:   derpMap,
		})
	})
	require.NoError(t, err)
	defer send.Close()
	sendCaptureStop := send.Capture(sendCapture)
	defer sendCaptureStop()

	recv.SetNodeCallback(updateNodes(send))
	send.SetNodeCallback(updateNodes(recv))

	ctx, cancel := context.WithTimeout(ctx, testutil.WaitLong)
	defer cancel()

	logger.Info(ctx, "waiting for receiver to be reachable (by sender)")
	require.True(t, send.AwaitReachable(ctx, recvIP))
	logger.Info(ctx, "wait complete")

	copyDone := make(chan struct{})
	go pprof.Do(ctx, pprof.Labels("id", "tailnet.recv.listener"), func(_ context.Context) {
		defer close(copyDone)

		ln, err := recv.Listen("tcp", ":35565")
		if !assert.NoError(t, err) {
			return
		}
		defer ln.Close()

		r, err := ln.Accept()
		if !assert.NoError(t, err) {
			return
		}
		defer r.Close()

		_, err = io.Copy(io.Discard, r)
		assert.NoError(t, err)
	})

	logger.Info(ctx, "dialing receiver")
	var w net.Conn
	pprof.Do(ctx, pprof.Labels("id", "tailnet.send.dial-recv"), func(ctx context.Context) {
		w, err = send.DialContextTCP(ctx, netip.AddrPortFrom(recvIP, 35565))
	})
	require.NoError(t, err)
	defer w.Close()
	logger.Info(ctx, "dial complete")

	now := time.Now()

	payload := []byte(strings.Repeat("hello world\n", 65536/12))
	size := 0
	retries := 0
	writeTimeout := 2 * time.Second
	goroutineGoodDumpWritten := false
	for i := 0; i < 1024*2; i++ {
		logger.Debug(ctx, "write payload", slog.F("num", i), slog.F("transmitted_kb", size/1024))
	Retry:
		n := 0
		pprof.Do(ctx, pprof.Labels("id", "tailnet.send.write", "iter", strconv.Itoa(i)), func(_ context.Context) {
			_ = w.SetWriteDeadline(time.Now().Add(writeTimeout))
			n, err = w.Write(payload)
		})
		if err != nil {
			if time.Duration(retries)*writeTimeout < timeout {
				var b bytes.Buffer
				_ = pprof.Lookup("goroutine").WriteTo(&b, 1)
				logger.Error(ctx, "write failed", slog.Error(err))
				_, _ = testLog.Write(b.Bytes())
				f, err := os.Create(filepath.Join(captureDir, fmt.Sprintf("goroutine-bad-%d.txt", i)))
				if err == nil {
					_, _ = f.Write(b.Bytes())
					_ = f.Close()
				}
				retries++
				logger.Info(ctx, "retrying", slog.F("try", retries))
				goto Retry
			} else {
				require.NoError(t, err)
			}
		} else if !goroutineGoodDumpWritten {
			f, err := os.Create(filepath.Join(captureDir, fmt.Sprintf("goroutine-good-%d.txt", i)))
			if err == nil {
				_ = pprof.Lookup("goroutine").WriteTo(f, 1)
				_ = f.Close()
				goroutineGoodDumpWritten = true
			}
		}
		size += n
	}

	err = w.Close()
	require.NoError(t, err)

	<-copyDone

	if time.Since(now) > timeout {
		t.Fatal("took too long to transmit")
	}
}
