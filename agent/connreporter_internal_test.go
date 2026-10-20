package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestConnectionReporter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expectedIP string
		report     proto.ConnectEvent
	}{
		{
			// Test that reporting doesn't choke if given an empty IP string, which is
			// what we send if we cannot get the remote address.
			name: "EmptyIP",
			report: proto.ConnectEvent{
				ID: uuid.New(),
			},
		},
		{
			name:       "WithIP",
			expectedIP: "127.0.0.1",
			report: proto.ConnectEvent{
				ID: uuid.New(),
				IP: "127.0.0.1:8080",
			},
		},
		{
			name:       "Localhost",
			expectedIP: "127.0.0.1",
			report: proto.ConnectEvent{
				ID: uuid.New(),
				IP: "localhost:8080",
			},
		},
		{
			name: "WithClientSessionID",
			report: proto.ConnectEvent{
				ID:              uuid.New(),
				ClientSessionID: "0123456789abcdef0123456789abcdef",
			},
		},
		{
			name: "WithMethodAndAppName",
			report: proto.ConnectEvent{
				ID:      uuid.New(),
				Method:  proto.Connection_METHOD_SSH,
				AppName: "cursor",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			logger := testutil.Logger(t)
			reporter := newConnectionReporter(ctx, logger)

			sink := newFakeConnectionReportSink()
			loopCtx, loopCancel := context.WithCancel(ctx)
			defer loopCancel()
			go func() {
				err := reporter.reportLoop(loopCtx, sink)
				assert.Error(t, err) // Loop canceled.
			}()

			connReporter := reporter.Connect(tc.report)

			req0 := testutil.RequireReceive(ctx, t, sink.report)
			require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req0.GetConnection().GetType())
			require.Equal(t, tc.expectedIP, req0.GetConnection().Ip)
			require.Equal(t, tc.report.ID[:], req0.GetConnection().GetId())
			require.Equal(t, proto.Connection_CONNECT, req0.GetConnection().GetAction())
			require.Equal(t, tc.report.ClientSessionID, req0.GetConnection().GetClientSessionId())
			require.Equal(t, tc.report.AppName, req0.GetConnection().GetAppName())
			require.Equal(t, tc.report.Method, req0.GetConnection().GetConnectionMethod())
			require.Equal(t, int64(0), req0.GetConnection().GetRxBytes())
			require.Equal(t, int64(0), req0.GetConnection().GetTxBytes())

			connReporter.Disconnect(proto.DisconnectEvent{
				Code:    0,
				Reason:  "because",
				RxBytes: 10,
				TxBytes: 20,
			})

			req1 := testutil.RequireReceive(ctx, t, sink.report)
			require.Equal(t, tc.expectedIP, req1.GetConnection().Ip)
			require.Equal(t, tc.report.ID[:], req1.GetConnection().GetId())
			require.Equal(t, proto.Connection_DISCONNECT, req1.GetConnection().GetAction())
			require.Equal(t, "because", req1.GetConnection().GetReason())
			require.Equal(t, tc.report.ClientSessionID, req1.GetConnection().GetClientSessionId())
			require.Equal(t, tc.report.AppName, req1.GetConnection().GetAppName())
			require.Equal(t, tc.report.Method, req1.GetConnection().GetConnectionMethod())
			require.Equal(t, int64(10), req1.GetConnection().GetRxBytes())
			require.Equal(t, int64(20), req1.GetConnection().GetTxBytes())
		})
	}

	t.Run("DropsPastLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := testutil.Logger(t)
		reporter := newConnectionReporter(ctx, logger)

		// Given: reports past the buffer limit.
		limit := 2
		reporter.report = make(chan *proto.ReportConnectionRequest, limit)
		for i := range limit {
			connReporter := reporter.Connect(proto.ConnectEvent{
				ID: uuid.New(),
			})
			connReporter.Disconnect(proto.DisconnectEvent{
				Code: i,
			})
		}

		// Then: should only get reports up to the limit.
		for i := range limit + 1 {
			var r *proto.ReportConnectionRequest
			select {
			case r = <-reporter.report:
			default:
			}

			if i < limit {
				require.NotNil(t, r, "should have received a report")
			} else {
				require.Nil(t, r, "report should have been dropped")
			}
		}
	})
}

func TestConnectionReporterWaitUntilReported(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	reporter := newConnectionReporter(ctx, testutil.Logger(t))
	sink := newFakeConnectionReportSink()
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = reporter.reportLoop(loopCtx, sink) }()

	require.NoError(t, reporter.waitUntilReported(ctx), "no connections are open")

	connection := reporter.Connect(proto.ConnectEvent{ID: uuid.New(), Method: proto.Connection_METHOD_SSH})
	waited := make(chan error, 1)
	go func() { waited <- reporter.waitUntilReported(ctx) }()
	requireWaiting := func() {
		t.Helper()
		select {
		case err := <-waited:
			t.Fatalf("wait finished early: %v", err)
		default:
		}
	}
	// The sink holds each report until the test receives it. The wait must
	// last until the open connection reports its disconnect and that report
	// has been sent.
	testutil.RequireReceive(ctx, t, sink.report)
	requireWaiting()
	connection.Disconnect(proto.DisconnectEvent{})
	connection.Disconnect(proto.DisconnectEvent{})
	requireWaiting()
	testutil.RequireReceive(ctx, t, sink.report)
	require.NoError(t, testutil.RequireReceive(ctx, t, waited))

	// An ended context stops the wait while a connection is still open.
	reporter.Connect(proto.ConnectEvent{ID: uuid.New(), Method: proto.Connection_METHOD_SSH})
	waitCtx, waitCancel := context.WithCancel(ctx)
	waitCancel()
	require.ErrorIs(t, reporter.waitUntilReported(waitCtx), context.Canceled)
	testutil.RequireReceive(ctx, t, sink.report)
}

type fakeConnectionReportSink struct {
	report chan *proto.ReportConnectionRequest
}

func newFakeConnectionReportSink() *fakeConnectionReportSink {
	return &fakeConnectionReportSink{
		report: make(chan *proto.ReportConnectionRequest),
	}
}

func (r *fakeConnectionReportSink) ReportConnection(_ context.Context, in *proto.ReportConnectionRequest) (*emptypb.Empty, error) {
	r.report <- in
	return &emptypb.Empty{}, nil
}
