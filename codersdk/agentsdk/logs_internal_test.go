package agentsdk
import (
	"errors"
	"context"
	"slices"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)
func TestLogSender_Mainline(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 0, src 1",
		Level:     codersdk.LogLevelInfo,
	})
	ls2 := uuid.UUID{0x22}
	uut.Enqueue(ls2,
		Log{
			CreatedAt: t0,
			Output:    "test log 0, src 2",
			Level:     codersdk.LogLevelError,
		},
		Log{
			CreatedAt: t0,
			Output:    "test log 1, src 2",
			Level:     codersdk.LogLevelWarn,
		},
	)
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	empty := make(chan error, 1)
	go func() {
		err := uut.WaitUntilEmpty(ctx)
		empty <- err
	}()
	// since neither source has even been flushed, it should immediately Flush
	// both, although the order is not controlled
	var logReqs []*proto.BatchCreateLogsRequest
	logReqs = append(logReqs, testutil.RequireRecvCtx(ctx, t, fDest.reqs))
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	logReqs = append(logReqs, testutil.RequireRecvCtx(ctx, t, fDest.reqs))
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	for _, req := range logReqs {
		require.NotNil(t, req)
		srcID, err := uuid.FromBytes(req.LogSourceId)
		require.NoError(t, err)
		switch srcID {
		case ls1:
			require.Len(t, req.Logs, 1)
			require.Equal(t, "test log 0, src 1", req.Logs[0].GetOutput())
			require.Equal(t, proto.Log_INFO, req.Logs[0].GetLevel())
			require.Equal(t, t0, req.Logs[0].GetCreatedAt().AsTime())
		case ls2:
			require.Len(t, req.Logs, 2)
			require.Equal(t, "test log 0, src 2", req.Logs[0].GetOutput())
			require.Equal(t, proto.Log_ERROR, req.Logs[0].GetLevel())
			require.Equal(t, t0, req.Logs[0].GetCreatedAt().AsTime())
			require.Equal(t, "test log 1, src 2", req.Logs[1].GetOutput())
			require.Equal(t, proto.Log_WARN, req.Logs[1].GetLevel())
			require.Equal(t, t0, req.Logs[1].GetCreatedAt().AsTime())
		default:
			t.Fatal("unknown log source")
		}
	}
	t1 := dbtime.Now()
	uut.Enqueue(ls1, Log{
		CreatedAt: t1,
		Output:    "test log 1, src 1",
		Level:     codersdk.LogLevelDebug,
	})
	uut.Flush(ls1)
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	// give ourselves a 25% buffer if we're right on the cusp of a tick
	require.LessOrEqual(t, time.Since(t1), flushInterval*5/4)
	require.NotNil(t, req)
	require.Len(t, req.Logs, 1)
	require.Equal(t, "test log 1, src 1", req.Logs[0].GetOutput())
	require.Equal(t, proto.Log_DEBUG, req.Logs[0].GetLevel())
	require.Equal(t, t1, req.Logs[0].GetCreatedAt().AsTime())
	err := testutil.RequireRecvCtx(ctx, t, empty)
	require.NoError(t, err)
	cancel()
	err = testutil.RequireRecvCtx(testCtx, t, loopErr)
	require.ErrorIs(t, err, context.Canceled)
	// we can still enqueue more logs after SendLoop returns
	uut.Enqueue(ls1, Log{
		CreatedAt: t1,
		Output:    "test log 2, src 1",
		Level:     codersdk.LogLevelTrace,
	})
}
func TestLogSender_LogLimitExceeded(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 0, src 1",
		Level:     codersdk.LogLevelInfo,
	})
	empty := make(chan error, 1)
	go func() {
		err := uut.WaitUntilEmpty(ctx)
		empty <- err
	}()
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	testutil.RequireSendCtx(ctx, t, fDest.resps,
		&proto.BatchCreateLogsResponse{LogLimitExceeded: true})
	err := testutil.RequireRecvCtx(ctx, t, loopErr)
	require.ErrorIs(t, err, LogLimitExceededError)
	// Should also unblock WaitUntilEmpty
	err = testutil.RequireRecvCtx(ctx, t, empty)
	require.NoError(t, err)
	// we can still enqueue more logs after SendLoop returns, but they don't
	// actually get enqueued
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 2, src 1",
		Level:     codersdk.LogLevelTrace,
	})
	uut.L.Lock()
	require.Len(t, uut.queues, 0)
	uut.L.Unlock()
	// Also, if we run SendLoop again, it should immediately exit.
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	err = testutil.RequireRecvCtx(ctx, t, loopErr)
	require.ErrorIs(t, err, LogLimitExceededError)
}
func TestLogSender_SkipHugeLog(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	// since we add some overhead to the actual length of the output, a log just
	// under the perBatch limit will not be accepted.
	hugeLog := make([]byte, maxBytesPerBatch-1)
	for i := range hugeLog {
		hugeLog[i] = 'q'
	}
	uut.Enqueue(ls1,
		Log{
			CreatedAt: t0,
			Output:    string(hugeLog),
			Level:     codersdk.LogLevelInfo,
		},
		Log{
			CreatedAt: t0,
			Output:    "test log 1, src 1",
			Level:     codersdk.LogLevelInfo,
		})
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	require.Len(t, req.Logs, 1, "it should skip the huge log")
	require.Equal(t, "test log 1, src 1", req.Logs[0].GetOutput())
	require.Equal(t, proto.Log_INFO, req.Logs[0].GetLevel())
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, loopErr)
	require.ErrorIs(t, err, context.Canceled)
}
func TestLogSender_InvalidUTF8(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	uut.Enqueue(ls1,
		Log{
			CreatedAt: t0,
			Output:    "test log 0, src 1\xc3\x28",
			Level:     codersdk.LogLevelInfo,
		},
		Log{
			CreatedAt: t0,
			Output:    "test log 1, src 1",
			Level:     codersdk.LogLevelInfo,
		})
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	require.Len(t, req.Logs, 2, "it should sanitize invalid UTF-8, but still send")
	// the 0xc3, 0x28 is an invalid 2-byte sequence in UTF-8.  The sanitizer replaces 0xc3 with ❌, and then
	// interprets 0x28 as a 1-byte sequence "("
	require.Equal(t, "test log 0, src 1❌(", req.Logs[0].GetOutput())
	require.Equal(t, proto.Log_INFO, req.Logs[0].GetLevel())
	require.Equal(t, "test log 1, src 1", req.Logs[1].GetOutput())
	require.Equal(t, proto.Log_INFO, req.Logs[1].GetLevel())
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, loopErr)
	require.ErrorIs(t, err, context.Canceled)
}
func TestLogSender_Batch(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	var logs []Log
	for i := 0; i < 60000; i++ {
		logs = append(logs, Log{
			CreatedAt: t0,
			Output:    "r",
			Level:     codersdk.LogLevelInfo,
		})
	}
	uut.Enqueue(ls1, logs...)
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	// with 60k logs, we should split into two updates to avoid going over 1MiB, since each log
	// is about 21 bytes.
	gotLogs := 0
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	gotLogs += len(req.Logs)
	wire, err := protobuf.Marshal(req)
	require.NoError(t, err)
	require.Less(t, len(wire), maxBytesPerBatch, "wire should not exceed 1MiB")
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	req = testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	gotLogs += len(req.Logs)
	wire, err = protobuf.Marshal(req)
	require.NoError(t, err)
	require.Less(t, len(wire), maxBytesPerBatch, "wire should not exceed 1MiB")
	require.Equal(t, 60000, gotLogs)
	testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	cancel()
	err = testutil.RequireRecvCtx(testCtx, t, loopErr)
	require.ErrorIs(t, err, context.Canceled)
}
func TestLogSender_MaxQueuedLogs(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	n := 4
	hugeLog := make([]byte, maxBytesQueued/n)
	for i := range hugeLog {
		hugeLog[i] = 'q'
	}
	var logs []Log
	for i := 0; i < n; i++ {
		logs = append(logs, Log{
			CreatedAt: t0,
			Output:    string(hugeLog),
			Level:     codersdk.LogLevelInfo,
		})
	}
	uut.Enqueue(ls1, logs...)
	// we're now right at the limit of output
	require.Equal(t, maxBytesQueued, uut.outputLen)
	// adding more logs should not error...
	ls2 := uuid.UUID{0x22}
	uut.Enqueue(ls2, logs...)
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	// It should still queue up one log from source #2, so that we would exceed the database
	// limit. These come over a total of 3 updates, because due to overhead, the n logs from source
	// #1 come in 2 updates, plus 1 update for source #2.
	logsBySource := make(map[uuid.UUID]int)
	for i := 0; i < 3; i++ {
		req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
		require.NotNil(t, req)
		srcID, err := uuid.FromBytes(req.LogSourceId)
		require.NoError(t, err)
		logsBySource[srcID] += len(req.Logs)
		testutil.RequireSendCtx(ctx, t, fDest.resps, &proto.BatchCreateLogsResponse{})
	}
	require.Equal(t, map[uuid.UUID]int{
		ls1: n,
		ls2: 1,
	}, logsBySource)
	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, loopErr)
	require.ErrorIs(t, err, context.Canceled)
}
func TestLogSender_SendError(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fDest := newFakeLogDest()
	expectedErr := errors.New("test")
	fDest.err = expectedErr
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 0, src 1",
		Level:     codersdk.LogLevelInfo,
	})
	loopErr := make(chan error, 1)
	go func() {
		err := uut.SendLoop(ctx, fDest)
		loopErr <- err
	}()
	req := testutil.RequireRecvCtx(ctx, t, fDest.reqs)
	require.NotNil(t, req)
	err := testutil.RequireRecvCtx(ctx, t, loopErr)
	require.ErrorIs(t, err, expectedErr)
	// we can still enqueue more logs after SendLoop returns
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 2, src 1",
		Level:     codersdk.LogLevelTrace,
	})
	uut.L.Lock()
	require.Len(t, uut.queues, 1)
	uut.L.Unlock()
}
func TestLogSender_WaitUntilEmpty_ContextExpired(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := testutil.Logger(t)
	uut := NewLogSender(logger)
	t0 := dbtime.Now()
	ls1 := uuid.UUID{0x11}
	uut.Enqueue(ls1, Log{
		CreatedAt: t0,
		Output:    "test log 0, src 1",
		Level:     codersdk.LogLevelInfo,
	})
	empty := make(chan error, 1)
	go func() {
		err := uut.WaitUntilEmpty(ctx)
		empty <- err
	}()
	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, empty)
	require.ErrorIs(t, err, context.Canceled)
}
type fakeLogDest struct {
	reqs  chan *proto.BatchCreateLogsRequest
	resps chan *proto.BatchCreateLogsResponse
	err   error
}
func (f fakeLogDest) BatchCreateLogs(ctx context.Context, req *proto.BatchCreateLogsRequest) (*proto.BatchCreateLogsResponse, error) {
	// clone the logs so that modifications the sender makes don't affect our tests.  In production
	// these would be serialized/deserialized so we don't have to worry too much.
	req.Logs = slices.Clone(req.Logs)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case f.reqs <- req:
		if f.err != nil {
			return nil, f.err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case resp := <-f.resps:
			return resp, nil
		}
	}
}
func newFakeLogDest() *fakeLogDest {
	return &fakeLogDest{
		reqs:  make(chan *proto.BatchCreateLogsRequest),
		resps: make(chan *proto.BatchCreateLogsResponse),
	}
}
