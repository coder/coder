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
				ID:   uuid.New(),
				Type: proto.Connection_TYPE_UNSPECIFIED,
			},
		},
		{
			name:       "WithIP",
			expectedIP: "127.0.0.1",
			report: proto.ConnectEvent{
				ID:   uuid.New(),
				IP:   "127.0.0.1:8080",
				Type: proto.Connection_TYPE_UNSPECIFIED,
			},
		},
		{
			name:       "Localhost",
			expectedIP: "127.0.0.1",
			report: proto.ConnectEvent{
				ID:   uuid.New(),
				IP:   "localhost:8080",
				Type: proto.Connection_TYPE_UNSPECIFIED,
			},
		},
		{
			name: "WithClientSessionID",
			report: proto.ConnectEvent{
				ID:              uuid.New(),
				Type:            proto.Connection_TYPE_UNSPECIFIED,
				ClientSessionID: "0123456789abcdef0123456789abcdef",
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

			req0 := <-sink.report
			require.Equal(t, tc.report.Type, req0.GetConnection().GetType())
			require.Equal(t, tc.expectedIP, req0.GetConnection().Ip)
			require.Equal(t, tc.report.ID[:], req0.GetConnection().GetId())
			require.Equal(t, proto.Connection_CONNECT, req0.GetConnection().GetAction())
			require.Equal(t, tc.report.ClientSessionID, req0.GetConnection().GetClientSessionId())

			connReporter.Disconnect(proto.DisconnectEvent{
				Code:   0,
				Reason: "because",
			})

			req1 := <-sink.report
			require.Equal(t, tc.report.Type, req1.GetConnection().GetType())
			require.Equal(t, tc.expectedIP, req1.GetConnection().Ip)
			require.Equal(t, tc.report.ID[:], req1.GetConnection().GetId())
			require.Equal(t, proto.Connection_DISCONNECT, req1.GetConnection().GetAction())
			require.Equal(t, "because", req1.GetConnection().GetReason())
			require.Equal(t, tc.report.ClientSessionID, req1.GetConnection().GetClientSessionId())
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
				ID:   uuid.New(),
				Type: proto.Connection_SSH,
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

			if i < 2 {
				require.NotNil(t, r, "should have received a report")
			} else {
				require.Nil(t, r, "report should have been dropped")
			}
		}
	})
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
