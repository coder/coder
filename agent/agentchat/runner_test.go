package agentchat_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentchat/chatexec"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type releaseCall struct {
	chatID      uuid.UUID
	leaseEpoch  int64
	finalStatus string
	errMsg      string
}

type renewCall struct {
	chatID     uuid.UUID
	leaseEpoch int64
}

type fakeCoordinationClient struct {
	mu sync.Mutex

	reportStatusCalls []bool
	reportStatusErr   error

	pollResponses []*proto.PollChatWorkResponse
	pollCalls     []int32

	acquireResponses map[uuid.UUID]*proto.AcquireChatLeaseResponse
	acquireErrors    map[uuid.UUID]error
	acquireCalls     []uuid.UUID

	renewResponses map[uuid.UUID]*proto.RenewChatLeaseResponse
	renewErrors    map[uuid.UUID]error
	renewCalls     []renewCall
	renewCallCh    chan renewCall

	releaseCalls  []releaseCall
	releaseCallCh chan releaseCall
	releaseErr    error
}

var _ agentchat.CoordinationClient = (*fakeCoordinationClient)(nil)

func newFakeCoordinationClient() *fakeCoordinationClient {
	return &fakeCoordinationClient{
		acquireResponses: make(map[uuid.UUID]*proto.AcquireChatLeaseResponse),
		acquireErrors:    make(map[uuid.UUID]error),
		renewResponses:   make(map[uuid.UUID]*proto.RenewChatLeaseResponse),
		renewErrors:      make(map[uuid.UUID]error),
		renewCallCh:      make(chan renewCall, 10),
		releaseCallCh:    make(chan releaseCall, 10),
	}
}

func (f *fakeCoordinationClient) ReportChatRunnerStatus(_ context.Context, in *proto.ReportChatRunnerStatusRequest) (*proto.ReportChatRunnerStatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reportStatusCalls = append(f.reportStatusCalls, in.GetReady())
	if f.reportStatusErr != nil {
		return nil, f.reportStatusErr
	}
	return &proto.ReportChatRunnerStatusResponse{}, nil
}

func (f *fakeCoordinationClient) PollChatWork(_ context.Context, in *proto.PollChatWorkRequest) (*proto.PollChatWorkResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pollCalls = append(f.pollCalls, in.GetMaxChats())
	if len(f.pollResponses) == 0 {
		return &proto.PollChatWorkResponse{}, nil
	}

	resp := f.pollResponses[0]
	f.pollResponses = f.pollResponses[1:]
	return resp, nil
}

func (f *fakeCoordinationClient) AcquireChatLease(_ context.Context, in *proto.AcquireChatLeaseRequest) (*proto.AcquireChatLeaseResponse, error) {
	chatID, err := uuid.FromBytes(in.GetChatId())
	if err != nil {
		return nil, xerrors.Errorf("parse acquire chat id: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.acquireCalls = append(f.acquireCalls, chatID)
	if err := f.acquireErrors[chatID]; err != nil {
		return nil, err
	}
	if resp := f.acquireResponses[chatID]; resp != nil {
		return resp, nil
	}
	return &proto.AcquireChatLeaseResponse{}, nil
}

func (f *fakeCoordinationClient) RenewChatLease(_ context.Context, in *proto.RenewChatLeaseRequest) (*proto.RenewChatLeaseResponse, error) {
	chatID, err := uuid.FromBytes(in.GetChatId())
	if err != nil {
		return nil, xerrors.Errorf("parse renew chat id: %w", err)
	}

	call := renewCall{chatID: chatID, leaseEpoch: in.GetLeaseEpoch()}

	f.mu.Lock()
	f.renewCalls = append(f.renewCalls, call)
	err = f.renewErrors[chatID]
	resp := f.renewResponses[chatID]
	f.mu.Unlock()

	select {
	case f.renewCallCh <- call:
	default:
	}

	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp, nil
	}
	return &proto.RenewChatLeaseResponse{Renewed: true}, nil
}

func (f *fakeCoordinationClient) ReleaseChatLease(_ context.Context, in *proto.ReleaseChatLeaseRequest) (*proto.ReleaseChatLeaseResponse, error) {
	chatID, err := uuid.FromBytes(in.GetChatId())
	if err != nil {
		return nil, xerrors.Errorf("parse release chat id: %w", err)
	}

	call := releaseCall{
		chatID:      chatID,
		leaseEpoch:  in.GetLeaseEpoch(),
		finalStatus: in.GetFinalStatus(),
		errMsg:      in.GetError(),
	}

	f.mu.Lock()
	f.releaseCalls = append(f.releaseCalls, call)
	err = f.releaseErr
	f.mu.Unlock()

	select {
	case f.releaseCallCh <- call:
	default:
	}

	if err != nil {
		return nil, err
	}
	return &proto.ReleaseChatLeaseResponse{}, nil
}

func (f *fakeCoordinationClient) reportStatusSnapshot() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]bool(nil), f.reportStatusCalls...)
}

func (f *fakeCoordinationClient) pollCallsSnapshot() []int32 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]int32(nil), f.pollCalls...)
}

func (f *fakeCoordinationClient) acquireCallsSnapshot() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]uuid.UUID(nil), f.acquireCalls...)
}

func (f *fakeCoordinationClient) releaseCallsSnapshot() []releaseCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]releaseCall(nil), f.releaseCalls...)
}

func (f *fakeCoordinationClient) waitRenewCall(ctx context.Context, t testing.TB) renewCall {
	t.Helper()
	return testutil.TryReceive(ctx, t, f.renewCallCh)
}

func (f *fakeCoordinationClient) waitReleaseCall(ctx context.Context, t testing.TB) releaseCall {
	t.Helper()
	return testutil.TryReceive(ctx, t, f.releaseCallCh)
}

type testExecutorFunc func(ctx context.Context, chatID uuid.UUID) error

func (fn testExecutorFunc) Execute(ctx context.Context, chatID uuid.UUID) error {
	return fn(ctx, chatID)
}

type fakeExecutor struct {
	mu sync.Mutex

	startCh    map[uuid.UUID]chan struct{}
	finishCh   map[uuid.UUID]chan error
	doneCh     map[uuid.UUID]chan error
	startCount map[uuid.UUID]int
}

var _ agentchat.Executor = (*fakeExecutor)(nil)

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{
		startCh:    make(map[uuid.UUID]chan struct{}),
		finishCh:   make(map[uuid.UUID]chan error),
		doneCh:     make(map[uuid.UUID]chan error),
		startCount: make(map[uuid.UUID]int),
	}
}

func (f *fakeExecutor) Execute(ctx context.Context, chatID uuid.UUID) error {
	started, finish, done := f.channels(chatID)

	f.mu.Lock()
	f.startCount[chatID]++
	f.mu.Unlock()

	select {
	case started <- struct{}{}:
	default:
	}

	var err error
	select {
	case err = <-finish:
	case <-ctx.Done():
		err = ctx.Err()
	}

	select {
	case done <- err:
	default:
	}

	return err
}

func (f *fakeExecutor) channels(chatID uuid.UUID) (started chan struct{}, finish chan error, done chan error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	started = f.startCh[chatID]
	if started == nil {
		started = make(chan struct{}, 1)
		f.startCh[chatID] = started
	}

	finish = f.finishCh[chatID]
	if finish == nil {
		finish = make(chan error, 1)
		f.finishCh[chatID] = finish
	}

	done = f.doneCh[chatID]
	if done == nil {
		done = make(chan error, 1)
		f.doneCh[chatID] = done
	}

	return started, finish, done
}

func (f *fakeExecutor) waitStarted(ctx context.Context, t testing.TB, chatID uuid.UUID) {
	t.Helper()
	started, _, _ := f.channels(chatID)
	testutil.TryReceive(ctx, t, started)
}

func (f *fakeExecutor) finish(chatID uuid.UUID, err error) {
	_, finish, _ := f.channels(chatID)
	finish <- err
}

func (f *fakeExecutor) waitDone(ctx context.Context, t testing.TB, chatID uuid.UUID) error {
	t.Helper()
	_, _, done := f.channels(chatID)
	return testutil.TryReceive(ctx, t, done)
}

func (f *fakeExecutor) startedCountFor(chatID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.startCount[chatID]
}

type loopTraps struct {
	poll      *quartz.Trap
	heartbeat *quartz.Trap
}

func newLoopTraps(mClock *quartz.Mock) *loopTraps {
	return &loopTraps{
		poll:      mClock.Trap().NewTicker("agentchat", "poll"),
		heartbeat: mClock.Trap().NewTicker("agentchat", "heartbeat"),
	}
}

func (t *loopTraps) release(ctx context.Context) {
	t.poll.MustWait(ctx).MustRelease(ctx)
	t.heartbeat.MustWait(ctx).MustRelease(ctx)
}

func (t *loopTraps) close() {
	t.poll.Close()
	t.heartbeat.Close()
}

func testLogger(t testing.TB) slog.Logger {
	t.Helper()
	return slogtest.Make(t, nil).Leveled(slog.LevelDebug)
}

func newTestRunner(t testing.TB, client *fakeCoordinationClient, executor agentchat.Executor, clock quartz.Clock, modify func(*agentchat.Options)) *agentchat.Runner {
	t.Helper()

	opts := agentchat.Options{
		Client:   client,
		Logger:   testLogger(t),
		Clock:    clock,
		Executor: executor,
	}
	if modify != nil {
		modify(&opts)
	}

	runner, err := agentchat.New(opts)
	require.NoError(t, err)
	return runner
}

func workItems(chatIDs ...uuid.UUID) *proto.PollChatWorkResponse {
	items := make([]*proto.ChatWorkItem, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		chatID := chatID
		items = append(items, &proto.ChatWorkItem{ChatId: chatID[:]})
	}
	return &proto.PollChatWorkResponse{WorkItems: items}
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	client := newFakeCoordinationClient()
	logger := testLogger(t)

	tests := []struct {
		name   string
		modify func(*agentchat.Options)
		want   string
	}{
		{
			name: "NilClient",
			modify: func(opts *agentchat.Options) {
				opts.Client = nil
			},
			want: "client is required",
		},
		{
			name: "ZeroLogger",
			modify: func(opts *agentchat.Options) {
				opts.Logger = slog.Logger{}
			},
			want: "logger is required",
		},
		{
			name: "NegativePollInterval",
			modify: func(opts *agentchat.Options) {
				opts.PollInterval = -time.Second
			},
			want: "poll interval -1s: must be positive",
		},
		{
			name: "NegativeHeartbeatInterval",
			modify: func(opts *agentchat.Options) {
				opts.HeartbeatInterval = -time.Second
			},
			want: "heartbeat interval -1s: must be positive",
		},
		{
			name: "NegativeMaxConcurrent",
			modify: func(opts *agentchat.Options) {
				opts.MaxConcurrent = -1
			},
			want: "max concurrent -1: must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := agentchat.Options{Client: client, Logger: logger}
			tt.modify(&opts)
			runner, err := agentchat.New(opts)
			require.Error(t, err)
			require.Nil(t, runner)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestNew_Defaults(t *testing.T) {
	t.Parallel()

	runner, err := agentchat.New(agentchat.Options{
		Client: newFakeCoordinationClient(),
		Logger: testLogger(t),
	})
	require.NoError(t, err)
	require.NotNil(t, runner)
}

func TestStart_ReportsReady(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	require.NoError(t, runner.Start(ctx))
	require.NoError(t, runner.Close())
	require.Equal(t, []bool{true, false}, client.reportStatusSnapshot())
}

func TestStart_Unimplemented(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	client.reportStatusErr = drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	require.NoError(t, runner.Start(ctx))
	require.NoError(t, runner.Close())
	require.Equal(t, []bool{true}, client.reportStatusSnapshot())
	require.Empty(t, client.pollCallsSnapshot())
	require.Empty(t, client.acquireCallsSnapshot())
	require.Empty(t, client.releaseCallsSnapshot())
}

func TestStart_ReadinessError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	client.reportStatusErr = xerrors.New("boom")
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	err := runner.Start(ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "report chat runner ready")
	require.ErrorContains(t, err, "boom")
	require.NoError(t, runner.Close())
	require.Equal(t, []bool{true}, client.reportStatusSnapshot())
}

func TestStart_DuplicateStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	require.NoError(t, runner.Start(ctx))

	err := runner.Start(ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "runner already started")
	require.NoError(t, runner.Close())
}

func TestPoll_DiscoversWork(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 7, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	acquireCalls := client.acquireCallsSnapshot()
	require.Equal(t, []uuid.UUID{chatID}, acquireCalls)

	executor.finish(chatID, nil)
	require.NoError(t, executor.waitDone(ctx, t, chatID))
	require.NoError(t, runner.Close())
}

func TestPoll_SkipsDuplicates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID, chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 3, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 2
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	require.Equal(t, []uuid.UUID{chatID}, client.acquireCallsSnapshot())

	executor.finish(chatID, nil)
	require.NoError(t, executor.waitDone(ctx, t, chatID))
	require.NoError(t, runner.Close())
}

func TestPoll_MaxConcurrent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID1 := uuid.New()
	chatID2 := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID1, chatID2)}
	client.acquireResponses[chatID1] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 11, Status: "waiting"}
	client.acquireResponses[chatID2] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 12, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID1)
	require.Equal(t, []int32{1}, client.pollCallsSnapshot())
	require.Equal(t, []uuid.UUID{chatID1}, client.acquireCallsSnapshot())
	require.Zero(t, executor.startedCountFor(chatID2))

	executor.finish(chatID1, nil)
	require.NoError(t, executor.waitDone(ctx, t, chatID1))
	require.NoError(t, runner.Close())
}

func TestPoll_InvalidChatID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{{
		WorkItems: []*proto.ChatWorkItem{{ChatId: []byte{}}},
	}}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	require.Empty(t, client.acquireCallsSnapshot())
	require.NoError(t, runner.Close())
}

func TestPoll_AcquireFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireErrors[chatID] = xerrors.New("acquire boom")
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	require.Equal(t, []uuid.UUID{chatID}, client.acquireCallsSnapshot())
	require.Zero(t, executor.startedCountFor(chatID))
	require.Empty(t, client.releaseCallsSnapshot())
	require.NoError(t, runner.Close())
}

func TestPoll_InvalidLeaseEpoch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 0, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	require.Equal(t, []uuid.UUID{chatID}, client.acquireCallsSnapshot())
	require.Zero(t, executor.startedCountFor(chatID))
	require.Empty(t, client.releaseCallsSnapshot())
	require.NoError(t, runner.Close())
}

func TestHeartbeat_RenewsActiveLeases(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	traps := newLoopTraps(mClock)
	defer traps.close()

	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 17, Status: "waiting"}
	client.renewResponses[chatID] = &proto.RenewChatLeaseResponse{Renewed: true}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, mClock, func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
		opts.PollInterval = time.Minute
		opts.HeartbeatInterval = time.Second
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	traps.release(ctx)
	mClock.Advance(time.Second).MustWait(ctx)
	renew := client.waitRenewCall(ctx, t)
	require.Equal(t, chatID, renew.chatID)
	require.Equal(t, int64(17), renew.leaseEpoch)

	executor.finish(chatID, nil)
	require.NoError(t, executor.waitDone(ctx, t, chatID))
	require.NoError(t, runner.Close())
}

func TestHeartbeat_RenewFalse_CancelsWorker(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	traps := newLoopTraps(mClock)
	defer traps.close()

	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 21, Status: "waiting"}
	client.renewResponses[chatID] = &proto.RenewChatLeaseResponse{Renewed: false}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, mClock, func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
		opts.PollInterval = time.Minute
		opts.HeartbeatInterval = time.Second
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	traps.release(ctx)
	mClock.Advance(time.Second).MustWait(ctx)
	require.ErrorIs(t, executor.waitDone(ctx, t, chatID), context.Canceled)
	require.NoError(t, runner.Close())
	require.Empty(t, client.releaseCallsSnapshot())
}

func TestHeartbeat_RenewError_CancelsWorker(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	traps := newLoopTraps(mClock)
	defer traps.close()

	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 25, Status: "waiting"}
	client.renewErrors[chatID] = xerrors.New("renew boom")
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, mClock, func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
		opts.PollInterval = time.Minute
		opts.HeartbeatInterval = time.Second
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	traps.release(ctx)
	mClock.Advance(time.Second).MustWait(ctx)
	require.ErrorIs(t, executor.waitDone(ctx, t, chatID), context.Canceled)
	require.NoError(t, runner.Close())
	require.Empty(t, client.releaseCallsSnapshot())
}

func TestExecutor_ErrorRelease(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	execErr := xerrors.New("executor boom")
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 31, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	executor.finish(chatID, execErr)
	release := client.waitReleaseCall(ctx, t)
	require.NoError(t, runner.Close())

	require.Equal(t, releaseCall{
		chatID:      chatID,
		leaseEpoch:  31,
		finalStatus: "error",
		errMsg:      execErr.Error(),
	}, release)
}

func TestExecutor_RequiresActionRelease(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 33, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	executor.finish(chatID, chatexec.ErrRequiresAction)
	release := client.waitReleaseCall(ctx, t)
	require.NoError(t, runner.Close())

	require.Equal(t, releaseCall{
		chatID:      chatID,
		leaseEpoch:  33,
		finalStatus: "requires_action",
		errMsg:      "",
	}, release)
}

func TestExecutor_RequiresActionWinsOverCanceledContext(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 34, Status: "waiting"}
	started := make(chan struct{}, 1)
	sink := testutil.NewFakeSink(t)
	executor := testExecutorFunc(func(ctx context.Context, chatID uuid.UUID) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return chatexec.ErrRequiresAction
	})
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.Logger = sink.Logger()
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	testutil.TryReceive(ctx, t, started)
	require.NoError(t, runner.Close())

	releases := client.releaseCallsSnapshot()
	require.Len(t, releases, 1)
	require.Equal(t, releaseCall{
		chatID:      chatID,
		leaseEpoch:  34,
		finalStatus: "requires_action",
		errMsg:      "",
	}, releases[0])
	warns := sink.Entries(func(entry slog.SinkEntry) bool {
		return entry.Level == slog.LevelWarn && entry.Message == "chat execution failed"
	})
	require.Empty(t, warns)
}

func TestExecutor_SuccessRelease(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	client := newFakeCoordinationClient()
	client.pollResponses = []*proto.PollChatWorkResponse{workItems(chatID)}
	client.acquireResponses[chatID] = &proto.AcquireChatLeaseResponse{LeaseEpoch: 35, Status: "waiting"}
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), func(opts *agentchat.Options) {
		opts.MaxConcurrent = 1
	})

	require.NoError(t, runner.Start(ctx))
	executor.waitStarted(ctx, t, chatID)
	executor.finish(chatID, nil)
	release := client.waitReleaseCall(ctx, t)
	require.NoError(t, runner.Close())

	require.Equal(t, releaseCall{
		chatID:      chatID,
		leaseEpoch:  35,
		finalStatus: "waiting",
		errMsg:      "",
	}, release)
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	require.NoError(t, runner.Start(ctx))
	require.NoError(t, runner.Close())
	require.NoError(t, runner.Close())
}

func TestClose_AfterUnimplementedStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeCoordinationClient()
	client.reportStatusErr = drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)
	executor := newFakeExecutor()
	runner := newTestRunner(t, client, executor, quartz.NewMock(t), nil)

	require.NoError(t, runner.Start(ctx))
	require.NoError(t, runner.Close())
	require.Equal(t, []bool{true}, client.reportStatusSnapshot())
}
