//nolint:paralleltest,testpackage,revive,gocritic
package audit

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

func TestSocketAuditor_AuditRequest_QueuesLog(t *testing.T) {
	t.Parallel()

	auditor := setupSocketAuditor(t)

	auditor.AuditRequest(Request{
		Method:  "GET",
		URL:     "https://example.com",
		Host:    "example.com",
		Allowed: true,
		Rule:    "allow-all",
	})

	select {
	case log := <-auditor.logCh:
		if log.Allowed != true {
			t.Errorf("expected Allowed=true, got %v", log.Allowed)
		}
		httpReq := log.GetHttpRequest()
		if httpReq == nil {
			t.Fatal("expected HttpRequest, got nil")
		}
		if httpReq.Method != "GET" {
			t.Errorf("expected Method=GET, got %s", httpReq.Method)
		}
		if httpReq.Url != "https://example.com" {
			t.Errorf("expected URL=https://example.com, got %s", httpReq.Url)
		}
		// Rule should be set for allowed requests
		if httpReq.MatchedRule != "allow-all" {
			t.Errorf("unexpected MatchedRule %v", httpReq.MatchedRule)
		}
	default:
		t.Fatal("expected log in channel, got none")
	}
}

func TestSocketAuditor_AuditRequest_AllowIncludesRule(t *testing.T) {
	t.Parallel()

	auditor := setupSocketAuditor(t)

	auditor.AuditRequest(Request{
		Method:  "POST",
		URL:     "https://evil.com",
		Host:    "evil.com",
		Allowed: true,
		Rule:    "allow-evil",
	})

	select {
	case log := <-auditor.logCh:
		if log.Allowed != true {
			t.Errorf("expected Allowed=false, got %v", log.Allowed)
		}
		httpReq := log.GetHttpRequest()
		if httpReq == nil {
			t.Fatal("expected HttpRequest, got nil")
		}
		if httpReq.MatchedRule != "allow-evil" {
			t.Errorf("expected MatchedRule=allow-evil, got %s", httpReq.MatchedRule)
		}
	default:
		t.Fatal("expected log in channel, got none")
	}
}

func TestSocketAuditor_AuditRequest_DropsWhenFull(t *testing.T) {
	t.Parallel()

	auditor := setupSocketAuditor(t)

	// Fill the channel (capacity is 2*batchSize = 20)
	for i := 0; i < 2*auditor.batchSize; i++ {
		auditor.AuditRequest(Request{Method: "GET", URL: "https://example.com", Allowed: true})
	}

	// This should not block and drop the log
	auditor.AuditRequest(Request{Method: "GET", URL: "https://dropped.com", Allowed: true})

	// Drain the channel and verify all entries are from the original batch (dropped.com was dropped)
	for i := 0; i < 2*auditor.batchSize; i++ {
		v := <-auditor.logCh
		resource, ok := v.Resource.(*agentproto.BoundaryLog_HttpRequest_)
		if !ok {
			t.Fatal("unexpected resource type")
		}
		if resource.HttpRequest.Url != "https://example.com" {
			t.Errorf("expected batch to be FIFO, got %s", resource.HttpRequest.Url)
		}
	}

	select {
	case v := <-auditor.logCh:
		t.Errorf("expected empty channel, got %v", v)
	default:
	}
}

func TestSocketAuditor_Loop_FlushesOnBatchSize(t *testing.T) {
	t.Parallel()

	auditor, serverConn := setupTestAuditor(t)
	auditor.batchTimerDuration = time.Hour // Ensure timer doesn't interfere with the test

	received := make(chan *agentproto.ReportBoundaryLogsRequest, 1)
	go readFromConn(t, serverConn, received)

	go auditor.Loop(t.Context())

	// Send exactly a full batch of logs to trigger a flush
	for i := 0; i < auditor.batchSize; i++ {
		auditor.AuditRequest(Request{Method: "GET", URL: "https://example.com", Allowed: true})
	}

	select {
	case req := <-received:
		if len(req.Logs) != auditor.batchSize {
			t.Errorf("expected %d logs, got %d", auditor.batchSize, len(req.Logs))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for flush")
	}
}

func TestSocketAuditor_Loop_FlushesOnTimer(t *testing.T) {
	t.Parallel()

	auditor, serverConn := setupTestAuditor(t)
	auditor.batchTimerDuration = 3 * time.Second

	received := make(chan *agentproto.ReportBoundaryLogsRequest, 1)
	go readFromConn(t, serverConn, received)

	go auditor.Loop(t.Context())

	// A single log should start the timer
	auditor.AuditRequest(Request{Method: "GET", URL: "https://example.com", Allowed: true})

	// Should flush after the timer duration elapses
	select {
	case req := <-received:
		if len(req.Logs) != 1 {
			t.Errorf("expected 1 log, got %d", len(req.Logs))
		}
	case <-time.After(2 * auditor.batchTimerDuration):
		t.Fatal("timeout waiting for timer flush")
	}
}

func TestSocketAuditor_Loop_FlushesOnContextCancel(t *testing.T) {
	t.Parallel()

	auditor, serverConn := setupTestAuditor(t)
	// Make the timer long to always exercise the context cancellation case
	auditor.batchTimerDuration = time.Hour

	received := make(chan *agentproto.ReportBoundaryLogsRequest, 1)
	go readFromConn(t, serverConn, received)

	ctx, cancel := context.WithCancel(t.Context())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		auditor.Loop(ctx)
	}()

	// Send a log but don't fill the batch
	auditor.AuditRequest(Request{Method: "GET", URL: "https://example.com", Allowed: true})

	cancel()

	select {
	case req := <-received:
		if len(req.Logs) != 1 {
			t.Errorf("expected 1 log, got %d", len(req.Logs))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown flush")
	}

	wg.Wait()
}

func TestSocketAuditor_Loop_RetriesOnConnectionFailure(t *testing.T) {
	t.Parallel()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		err := clientConn.Close()
		if err != nil {
			t.Errorf("close client connection: %v", err)
		}
		err = serverConn.Close()
		if err != nil {
			t.Errorf("close server connection: %v", err)
		}
	})

	var dialCount atomic.Int32
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditor := &SocketAuditor{
		dial: func() (net.Conn, error) {
			// First dial attempt fails, subsequent ones succeed
			if dialCount.Add(1) == 1 {
				return nil, xerrors.New("connection refused")
			}
			return clientConn, nil
		},
		logger:             logger,
		logCh:              make(chan *agentproto.BoundaryLog, 2*defaultBatchSize),
		batchSize:          defaultBatchSize,
		batchTimerDuration: time.Hour, // Ensure timer doesn't interfere with the test
	}

	// Set up hook to detect flush attempts
	flushed := make(chan struct{}, 1)
	auditor.onFlushAttempt = func() {
		select {
		case flushed <- struct{}{}:
		default:
		}
	}

	received := make(chan *agentproto.ReportBoundaryLogsRequest, 1)
	go readFromConn(t, serverConn, received)

	go auditor.Loop(t.Context())

	// Send batchSize+1 logs so we can verify the last log here gets dropped.
	for i := 0; i < auditor.batchSize+1; i++ {
		auditor.AuditRequest(Request{Method: "GET", URL: "https://servernotup.com", Allowed: true})
	}

	// Wait for the first flush attempt (which will fail)
	select {
	case <-flushed:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first flush attempt")
	}

	// Send one more log - batch is at capacity, so this triggers flush first
	// The flush succeeds (dial now works), sending the retained batch.
	auditor.AuditRequest(Request{Method: "POST", URL: "https://serverup.com", Allowed: true})

	// Should receive the retained batch (the new log goes into a fresh batch)
	select {
	case req := <-received:
		if len(req.Logs) != auditor.batchSize {
			t.Errorf("expected %d logs from retry, got %d", auditor.batchSize, len(req.Logs))
		}
		for _, log := range req.Logs {
			resource, ok := log.Resource.(*agentproto.BoundaryLog_HttpRequest_)
			if !ok {
				t.Fatal("unexpected resource type")
			}
			if resource.HttpRequest.Url != "https://servernotup.com" {
				t.Errorf("expected URL https://servernotup.com, got %v", resource.HttpRequest.Url)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for retry flush")
	}
}

func TestFlush_EmptyBatch(t *testing.T) {
	t.Parallel()

	err := flush(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for empty batch, got %v", err)
	}

	err = flush(nil, []*agentproto.BoundaryLog{})
	if err != nil {
		t.Errorf("expected nil error for empty slice, got %v", err)
	}
}

// setupSocketAuditor creates a SocketAuditor for tests that only exercise
// the queueing behavior (no connection needed).
func setupSocketAuditor(t *testing.T) *SocketAuditor {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &SocketAuditor{
		dial: func() (net.Conn, error) {
			return nil, xerrors.New("not connected")
		},
		logger:             logger,
		logCh:              make(chan *agentproto.BoundaryLog, 2*defaultBatchSize),
		batchSize:          defaultBatchSize,
		batchTimerDuration: defaultBatchTimerDuration,
	}
}

// setupTestAuditor creates a SocketAuditor with an in-memory connection using
// net.Pipe(). Returns the auditor and the server-side connection for reading.
func setupTestAuditor(t *testing.T) (*SocketAuditor, net.Conn) {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		err := clientConn.Close()
		if err != nil {
			t.Error("Failed to close client connection", "error", err)
		}
		err = serverConn.Close()
		if err != nil {
			t.Error("Failed to close server connection", "error", err)
		}
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditor := &SocketAuditor{
		dial: func() (net.Conn, error) {
			return clientConn, nil
		},
		logger:             logger,
		logCh:              make(chan *agentproto.BoundaryLog, 2*defaultBatchSize),
		batchSize:          defaultBatchSize,
		batchTimerDuration: defaultBatchTimerDuration,
	}

	return auditor, serverConn
}

// readFromConn reads length-prefixed protobuf messages from a connection and
// sends them to the received channel.
func readFromConn(t *testing.T, conn net.Conn, received chan<- *agentproto.ReportBoundaryLogsRequest) {
	t.Helper()

	buf := make([]byte, 1<<10)
	for {
		tag, data, err := codec.ReadFrame(conn, buf)
		if err != nil {
			return // connection closed
		}

		if tag != codec.TagV1 {
			t.Errorf("invalid tag: %d", tag)
		}

		var req agentproto.ReportBoundaryLogsRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
			return
		}

		received <- &req
	}
}
