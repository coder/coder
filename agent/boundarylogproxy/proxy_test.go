//go:build linux || darwin

package boundarylogproxy_test

import (
	"context"
	"encoding/binary"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/agent/boundarylogproxy"
	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
)

// sendLogsV1 writes a bare ReportBoundaryLogsRequest using TagV1, the
// legacy framing that existing boundary deployments use.
func sendLogsV1(t *testing.T, conn net.Conn, req *agentproto.ReportBoundaryLogsRequest) {
	t.Helper()

	err := codec.WriteMessage(conn, codec.TagV1, req)
	if err != nil {
		t.Errorf("write v1 logs: %s", err)
	}
}

// sendLogs writes a BoundaryMessage envelope containing logs to the
// connection using TagV2.
func sendLogs(t *testing.T, conn net.Conn, req *agentproto.ReportBoundaryLogsRequest) {
	t.Helper()

	msg := &codec.BoundaryMessage{
		Msg: &codec.BoundaryMessage_Logs{Logs: req},
	}
	err := codec.WriteMessage(conn, codec.TagV2, msg)
	if err != nil {
		t.Errorf("write logs: %s", err)
	}
}

// sendStatus writes a BoundaryMessage envelope containing a BoundaryStatus
// to the connection using TagV2.
func sendStatus(t *testing.T, conn net.Conn, status *codec.BoundaryStatus) {
	t.Helper()

	msg := &codec.BoundaryMessage{
		Msg: &codec.BoundaryMessage_Status{Status: status},
	}
	err := codec.WriteMessage(conn, codec.TagV2, msg)
	if err != nil {
		t.Errorf("write status: %s", err)
	}
}

// fakeReporter implements boundarylogproxy.Reporter for testing.
type fakeReporter struct {
	mu      sync.Mutex
	logs    []*agentproto.ReportBoundaryLogsRequest
	err     error
	errOnce bool // only error once, then succeed

	// reportCb is called when a ReportBoundaryLogsRequest is processed. It must not
	// block.
	reportCb func()
}

func (f *fakeReporter) ReportBoundaryLogs(_ context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.reportCb != nil {
		f.reportCb()
	}

	if f.err != nil {
		if f.errOnce {
			err := f.err
			f.err = nil
			return nil, err
		}
		return nil, f.err
	}
	f.logs = append(f.logs, req)
	return &agentproto.ReportBoundaryLogsResponse{}, nil
}

func (f *fakeReporter) getLogs() []*agentproto.ReportBoundaryLogsRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*agentproto.ReportBoundaryLogsRequest{}, f.logs...)
}

func TestServer_StartAndClose(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)

	// Verify socket exists and is connectable.
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	err = conn.Close()
	require.NoError(t, err)

	err = srv.Close()
	require.NoError(t, err)
}

func TestServer_ReceiveAndForwardLogs(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	// Start forwarder in background.
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	// Connect and send a log message.
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com",
					},
				},
			},
		},
	}

	sendLogs(t, conn, req)

	// Wait for the reporter to receive the log.
	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	logs := reporter.getLogs()
	require.Len(t, logs, 1)
	require.Len(t, logs[0].Logs, 1)
	require.True(t, logs[0].Logs[0].Allowed)
	require.Equal(t, "GET", logs[0].Logs[0].GetHttpRequest().Method)
	require.Equal(t, "https://example.com", logs[0].Logs[0].GetHttpRequest().Url)

	cancel()
	<-forwarderDone
}

func TestServer_MultipleMessages(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start()
	require.NoError(t, err)
	defer srv.Close()

	reporter := &fakeReporter{}

	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send multiple messages and verify they are all received.
	for range 5 {
		req := &agentproto.ReportBoundaryLogsRequest{
			Logs: []*agentproto.BoundaryLog{
				{
					Allowed: true,
					Time:    timestamppb.Now(),
					Resource: &agentproto.BoundaryLog_HttpRequest_{
						HttpRequest: &agentproto.BoundaryLog_HttpRequest{
							Method: "POST",
							Url:    "https://example.com/api",
						},
					},
				},
			},
		}
		sendLogs(t, conn, req)
	}

	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == 5
	}, testutil.WaitShort, testutil.IntervalFast)

	cancel()
	<-forwarderDone
}

func TestServer_MultipleConnections(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	// Create multiple connections and send from each.
	const numConns = 3
	var wg sync.WaitGroup
	wg.Add(numConns)
	for i := range numConns {
		go func(connID int) {
			defer wg.Done()
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Errorf("conn %d dial: %s", connID, err)
			}
			defer conn.Close()

			req := &agentproto.ReportBoundaryLogsRequest{
				Logs: []*agentproto.BoundaryLog{
					{
						Allowed: true,
						Time:    timestamppb.Now(),
						Resource: &agentproto.BoundaryLog_HttpRequest_{
							HttpRequest: &agentproto.BoundaryLog_HttpRequest{
								Method: "GET",
								Url:    "https://example.com",
							},
						},
					},
				},
			}
			sendLogs(t, conn, req)
		}(i)
	}
	wg.Wait()

	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == numConns
	}, testutil.WaitShort, testutil.IntervalFast)

	cancel()
	<-forwarderDone
}

func TestServer_MessageTooLarge(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send a message claiming to be larger than the max message size.
	var length uint32 = codec.MaxMessageSizeV1 + 1
	err = binary.Write(conn, binary.BigEndian, length)
	require.NoError(t, err)

	// The server should close the connection after receiving an oversized
	// message length.
	buf := make([]byte, 1)
	err = conn.SetReadDeadline(time.Now().Add(time.Second))
	require.NoError(t, err)
	_, err = conn.Read(buf)
	require.Error(t, err) // Should get EOF or closed connection.
}

func TestServer_ForwarderContinuesAfterError(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reportNotify := make(chan struct{}, 1)
	reporter := &fakeReporter{
		// Simulate an error on the first call.
		err:     context.DeadlineExceeded,
		errOnce: true,
		reportCb: func() {
			reportNotify <- struct{}{}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send the first message to be processed and wait for failure.
	req1 := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com/first",
					},
				},
			},
		},
	}
	sendLogs(t, conn, req1)

	select {
	case <-reportNotify:
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for first message to be processed")
	}

	// Send the second message, which should succeed.
	req2 := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: false,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "POST",
						Url:    "https://example.com/second",
					},
				},
			},
		},
	}
	sendLogs(t, conn, req2)

	// Only the second message should be recorded.
	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	logs := reporter.getLogs()
	require.Len(t, logs, 1)
	require.Equal(t, "https://example.com/second", logs[0].Logs[0].GetHttpRequest().Url)

	cancel()
	<-forwarderDone
}

func TestServer_CloseStopsForwarder(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	forwarderCtx, forwarderCancel := context.WithCancel(context.Background())
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(forwarderCtx, reporter)
	}()

	// Cancel the forwarder context and verify it stops.
	forwarderCancel()

	select {
	case err := <-forwarderDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(testutil.WaitShort):
		t.Fatal("forwarder did not stop")
	}
}

func TestServer_InvalidProtobuf(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send a valid header with garbage protobuf data.
	// The server should log an unmarshal error but continue processing.
	invalidProto := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	//nolint: gosec // codec.DataLength is always less than the size of the header.
	header := (uint32(codec.TagV1) << codec.DataLength) | uint32(len(invalidProto))
	err = binary.Write(conn, binary.BigEndian, header)
	require.NoError(t, err)
	_, err = conn.Write(invalidProto)
	require.NoError(t, err)

	// Now send a valid message. The server should continue processing.
	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com/valid",
					},
				},
			},
		},
	}
	sendLogs(t, conn, req)

	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	cancel()
	<-forwarderDone
}

func TestServer_InvalidHeader(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	// sendInvalidHeader sends a header and verifies the server closes the
	// connection.
	sendInvalidHeader := func(t *testing.T, name string, header uint32) {
		t.Helper()

		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		defer conn.Close()

		err = binary.Write(conn, binary.BigEndian, header)
		require.NoError(t, err, name)

		// The server closes the connection on invalid header, so the next
		// write should fail with a broken pipe error.
		require.Eventually(t, func() bool {
			_, err := conn.Write([]byte{0x00})
			return err != nil
		}, testutil.WaitShort, testutil.IntervalFast, name)
	}

	// TagV1 with length exceeding MaxMessageSizeV1.
	sendInvalidHeader(t, "v1 too large", (uint32(codec.TagV1)<<codec.DataLength)|(codec.MaxMessageSizeV1+1))

	// Unknown tag.
	const bogusTag = 0xFF
	sendInvalidHeader(t, "unknown tag too large", (bogusTag<<codec.DataLength)|(codec.MaxMessageSizeV1+1))

	cancel()
	<-forwarderDone
}

func TestServer_AllowRequest(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send an allowed request with a matched rule.
	logTime := timestamppb.Now()
	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    logTime,
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method:      "GET",
						Url:         "https://malicious.com/attack",
						MatchedRule: "*.malicious.com",
					},
				},
			},
		},
	}
	sendLogs(t, conn, req)

	require.Eventually(t, func() bool {
		logs := reporter.getLogs()
		return len(logs) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	logs := reporter.getLogs()
	require.Len(t, logs, 1)
	require.True(t, logs[0].Logs[0].Allowed)
	require.Equal(t, logTime.Seconds, logs[0].Logs[0].Time.Seconds)
	require.Equal(t, logTime.Nanos, logs[0].Logs[0].Time.Nanos)
	require.Equal(t, "*.malicious.com", logs[0].Logs[0].GetHttpRequest().MatchedRule)

	cancel()
	<-forwarderDone
}

func TestServer_TagV1BackwardsCompatibility(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
	srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	reporter := &fakeReporter{}

	forwarderDone := make(chan error, 1)
	go func() {
		forwarderDone <- srv.RunForwarder(ctx, reporter)
	}()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Send a TagV1 message (bare ReportBoundaryLogsRequest) to verify
	// the server still handles the legacy framing used by existing
	// boundary deployments.
	v1Req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com/v1",
					},
				},
			},
		},
	}
	sendLogsV1(t, conn, v1Req)

	require.Eventually(t, func() bool {
		return len(reporter.getLogs()) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Now send a TagV2 message on the same connection to verify both
	// tag versions work interleaved.
	v2Req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: false,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "POST",
						Url:    "https://example.com/v2",
					},
				},
			},
		},
	}
	sendLogs(t, conn, v2Req)

	require.Eventually(t, func() bool {
		return len(reporter.getLogs()) == 2
	}, testutil.WaitShort, testutil.IntervalFast)

	logs := reporter.getLogs()
	require.Equal(t, "https://example.com/v1", logs[0].Logs[0].GetHttpRequest().Url)
	require.Equal(t, "https://example.com/v2", logs[1].Logs[0].GetHttpRequest().Url)

	cancel()
	<-forwarderDone
}

func TestServer_Metrics(t *testing.T) {
	t.Parallel()

	makeReq := func(n int) *agentproto.ReportBoundaryLogsRequest {
		logs := make([]*agentproto.BoundaryLog, n)
		for i := range n {
			logs[i] = &agentproto.BoundaryLog{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://example.com",
					},
				},
			}
		}
		return &agentproto.ReportBoundaryLogsRequest{Logs: logs}
	}

	// BufferFull needs its own setup because it intentionally does not run
	// a forwarder so the channel fills up.
	t.Run("BufferFull", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
		srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, reg)

		err := srv.Start()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, srv.Close()) })

		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		defer conn.Close()

		// Fill the buffer (size 100) without running a forwarder so nothing
		// drains. Then send one more to trigger the drop path.
		for range 101 {
			sendLogs(t, conn, makeReq(1))
		}

		require.Eventually(t, func() bool {
			return getCounterVecValue(t, reg, "agent_boundary_log_proxy_batches_dropped_total", "buffer_full") >= 1
		}, testutil.WaitShort, testutil.IntervalFast)
		require.GreaterOrEqual(t,
			getCounterVecValue(t, reg, "agent_boundary_log_proxy_logs_dropped_total", "buffer_full"),
			float64(1))
	})

	// The remaining metrics share one server, forwarder, and connection. The
	// phases run sequentially so metrics accumulate.
	t.Run("Forwarding", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "boundary.sock")
		srv := boundarylogproxy.NewServer(testutil.Logger(t), socketPath, reg)

		err := srv.Start()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, srv.Close()) })

		reportNotify := make(chan struct{}, 4)
		reporter := &fakeReporter{
			err:     context.DeadlineExceeded,
			errOnce: true,
			reportCb: func() {
				select {
				case reportNotify <- struct{}{}:
				default:
				}
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		forwarderDone := make(chan error, 1)
		go func() {
			forwarderDone <- srv.RunForwarder(ctx, reporter)
		}()

		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		defer conn.Close()

		// Phase 1: the first forward errors
		sendLogs(t, conn, makeReq(2))

		select {
		case <-reportNotify:
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for forward attempt")
		}

		// The metric is incremented after ReportBoundaryLogs returns, so we
		// need to poll briefly.
		require.Eventually(t, func() bool {
			return getCounterVecValue(t, reg, "agent_boundary_log_proxy_batches_dropped_total", "forward_failed") >= 1
		}, testutil.WaitShort, testutil.IntervalFast)
		require.Equal(t, float64(2),
			getCounterVecValue(t, reg, "agent_boundary_log_proxy_logs_dropped_total", "forward_failed"))

		// Phase 2: forward succeeds.
		sendLogs(t, conn, makeReq(1))

		require.Eventually(t, func() bool {
			return len(reporter.getLogs()) >= 1
		}, testutil.WaitShort, testutil.IntervalFast)
		require.Equal(t, float64(1),
			getCounterValue(t, reg, "agent_boundary_log_proxy_batches_forwarded_total"))

		// Phase 3: boundary-reported drop counts arrive as a separate BoundaryStatus
		// message, not piggybacked on log batches.
		sendStatus(t, conn, &codec.BoundaryStatus{
			DroppedChannelFull: 5,
			DroppedBatchFull:   3,
		})

		// Status is handled immediately by the reader goroutine, not by the
		// forwarder, so poll metrics directly.
		require.Eventually(t, func() bool {
			return getCounterVecValue(t, reg, "agent_boundary_log_proxy_logs_dropped_total", "boundary_channel_full") >= 5
		}, testutil.WaitShort, testutil.IntervalFast)
		require.Equal(t, float64(5),
			getCounterVecValue(t, reg, "agent_boundary_log_proxy_logs_dropped_total", "boundary_channel_full"))
		require.Equal(t, float64(3),
			getCounterVecValue(t, reg, "agent_boundary_log_proxy_logs_dropped_total", "boundary_batch_full"))

		cancel()
		<-forwarderDone
	})
}

// getCounterVecValue returns the current value of a CounterVec metric filtered
// by the given reason label.
func getCounterVecValue(t *testing.T, reg *prometheus.Registry, name, reason string) float64 {
	t.Helper()

	metrics, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range metrics {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "reason" && lp.GetValue() == reason {
					return m.GetCounter().GetValue()
				}
			}
		}
	}

	return 0
}

// getCounterValue returns the current value of a Counter metric.
func getCounterValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()

	metrics, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range metrics {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			return m.GetCounter().GetValue()
		}
	}

	return 0
}
