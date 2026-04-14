package cli

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	ipnstate "tailscale.com/ipn/ipnstate"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_parsePortForwards(t *testing.T) {
	t.Parallel()

	type args struct {
		tcpSpecs []string
		udpSpecs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []portForwardSpec
		wantErr bool
	}{
		{
			name: "TCP mixed ports and ranges",
			args: args{
				tcpSpecs: []string{
					"8000,8080:8081,9000-9002,9003-9004:9005-9006",
					"10000",
					"4444-4444",
				},
			},
			want: []portForwardSpec{
				{"tcp", noAddr, 8000, 8000},
				{"tcp", noAddr, 8080, 8081},
				{"tcp", noAddr, 9000, 9000},
				{"tcp", noAddr, 9001, 9001},
				{"tcp", noAddr, 9002, 9002},
				{"tcp", noAddr, 9003, 9005},
				{"tcp", noAddr, 9004, 9006},
				{"tcp", noAddr, 10000, 10000},
				{"tcp", noAddr, 4444, 4444},
			},
		},
		{
			name: "TCP IPv4 local",
			args: args{
				tcpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", ipv4Loopback, 8080, 8081},
			},
		},
		{
			name: "TCP IPv6 local",
			args: args{
				tcpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"tcp", ipv6Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP with port range",
			args: args{
				udpSpecs: []string{"8000,8080-8081"},
			},
			want: []portForwardSpec{
				{"udp", noAddr, 8000, 8000},
				{"udp", noAddr, 8080, 8080},
				{"udp", noAddr, 8081, 8081},
			},
		},
		{
			name: "UDP IPv4 local",
			args: args{
				udpSpecs: []string{"127.0.0.1:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", ipv4Loopback, 8080, 8081},
			},
		},
		{
			name: "UDP IPv6 local",
			args: args{
				udpSpecs: []string{"[::1]:8080:8081"},
			},
			want: []portForwardSpec{
				{"udp", ipv6Loopback, 8080, 8081},
			},
		},
		{
			name: "Bad port range",
			args: args{
				tcpSpecs: []string{"8000-7000"},
			},
			wantErr: true,
		},
		{
			name: "Bad dest port range",
			args: args{
				tcpSpecs: []string{"8080-8081:9080-9082"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePortForwards(tt.args.tcpSpecs, tt.args.udpSpecs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePortForwards() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_agentHeartbeat(t *testing.T) {
	t.Parallel()

	const (
		interval    = 5 * time.Second
		timeout     = 10 * time.Second
		maxFailures = 3
	)

	// startHeartbeat launches agentHeartbeat in a goroutine and waits
	// for the TickerFunc to be registered with the mock clock before
	// returning. This ensures AdvanceNext will find the ticker.
	startHeartbeat := func(
		ctx context.Context,
		mock *agentconnmock.MockAgentConn,
		logger slog.Logger,
		mClock *quartz.Mock,
	) <-chan error {
		trap := mClock.Trap().TickerFunc("agentHeartbeat")
		errCh := make(chan error, 1)
		go func() {
			errCh <- agentHeartbeat(ctx, mock, logger, mClock, interval, timeout, maxFailures)
		}()
		trap.MustWait(ctx).Release(ctx)
		trap.Close()
		return errCh
	}

	t.Run("ReturnsErrAfterConsecutiveFailures", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mock := agentconnmock.NewMockAgentConn(ctrl)
		logger := slogtest.Make(t, nil)
		mClock := quartz.NewMock(t)

		mock.EXPECT().
			Ping(gomock.Any()).
			Return(time.Duration(0), false, &ipnstate.PingResult{}, assert.AnError).
			Times(maxFailures)

		ctx := testutil.Context(t, testutil.WaitShort)
		errCh := startHeartbeat(ctx, mock, logger, mClock)

		for range maxFailures {
			_, w := mClock.AdvanceNext()
			w.MustWait(ctx)
		}

		err := testutil.TryReceive(ctx, t, errCh)
		require.ErrorIs(t, err, assert.AnError)
		require.Contains(t, err.Error(), "consecutive failed pings")
	})

	t.Run("ResetsCounterOnSuccess", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mock := agentconnmock.NewMockAgentConn(ctrl)
		logger := slogtest.Make(t, nil)
		mClock := quartz.NewMock(t)

		// Fail twice, succeed once (resets counter), then fail three
		// more times to trigger the error.
		gomock.InOrder(
			mock.EXPECT().Ping(gomock.Any()).
				Return(time.Duration(0), false, &ipnstate.PingResult{}, assert.AnError).
				Times(2),
			mock.EXPECT().Ping(gomock.Any()).
				Return(time.Millisecond, true, &ipnstate.PingResult{}, nil).
				Times(1),
			mock.EXPECT().Ping(gomock.Any()).
				Return(time.Duration(0), false, &ipnstate.PingResult{}, assert.AnError).
				Times(maxFailures),
		)

		ctx := testutil.Context(t, testutil.WaitShort)
		errCh := startHeartbeat(ctx, mock, logger, mClock)

		// 2 failures + 1 success + 3 failures = 6 ticks.
		for range 2 + 1 + maxFailures {
			_, w := mClock.AdvanceNext()
			w.MustWait(ctx)
		}

		err := testutil.TryReceive(ctx, t, errCh)
		require.ErrorIs(t, err, assert.AnError)
		require.Contains(t, err.Error(), "consecutive failed pings")
	})

	t.Run("ReturnsNilOnContextCancel", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mock := agentconnmock.NewMockAgentConn(ctrl)
		logger := slogtest.Make(t, nil)
		mClock := quartz.NewMock(t)

		testCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(testCtx)

		mock.EXPECT().
			Ping(gomock.Any()).
			Return(time.Millisecond, true, &ipnstate.PingResult{}, nil).
			AnyTimes()

		errCh := startHeartbeat(ctx, mock, logger, mClock)

		// A few successful pings, then cancel.
		for range 3 {
			_, w := mClock.AdvanceNext()
			w.MustWait(testCtx)
		}
		cancel()

		err := testutil.TryReceive(testCtx, t, errCh)
		require.NoError(t, err)
	})

	t.Run("ContextCancelDuringFailureIsNotError", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mock := agentconnmock.NewMockAgentConn(ctrl)
		logger := slogtest.Make(t, nil)
		mClock := quartz.NewMock(t)

		testCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(testCtx)

		callCount := 0
		mock.EXPECT().
			Ping(gomock.Any()).
			DoAndReturn(func(_ context.Context) (time.Duration, bool, *ipnstate.PingResult, error) {
				callCount++
				if callCount >= 2 {
					cancel()
				}
				return time.Duration(0), false, &ipnstate.PingResult{}, assert.AnError
			}).
			AnyTimes()

		errCh := startHeartbeat(ctx, mock, logger, mClock)

		// Advance twice — second ping cancels the context.
		for range 2 {
			_, w := mClock.AdvanceNext()
			w.MustWait(testCtx)
		}

		err := testutil.TryReceive(testCtx, t, errCh)
		require.NoError(t, err)
	})
}
