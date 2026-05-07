package chatd //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestDialWithLazyValidation_FastDial(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	conn := agentconnmock.NewMockAgentConn(ctrl)

	var releaseCalls atomic.Int32
	var validateCalls atomic.Int32

	result, err := dialWithLazyValidation(
		context.Background(),
		agentID,
		workspaceID,
		func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			return conn, func() {
				releaseCalls.Add(1)
			}, nil
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			validateCalls.Add(1)
			return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
		},
		time.Minute,
	)
	require.NoError(t, err)
	require.Same(t, conn, result.Conn)
	require.Equal(t, agentID, result.AgentID)
	require.False(t, result.WasSwitched)
	require.EqualValues(t, 0, validateCalls.Load())

	if result.Release != nil {
		result.Release()
	}
	require.EqualValues(t, 1, releaseCalls.Load())
}

func TestDialWithLazyValidation_SlowDialSameAgent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	conn := agentconnmock.NewMockAgentConn(ctrl)
	unblockDial := make(chan struct{})

	var releaseCalls atomic.Int32
	var validateCalls atomic.Int32

	result, err := dialWithLazyValidation(
		context.Background(),
		agentID,
		workspaceID,
		func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			select {
			case <-unblockDial:
				return conn, func() {
					releaseCalls.Add(1)
				}, nil
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			close(unblockDial)
			return agentID, nil
		},
		0,
	)
	require.NoError(t, err)
	require.Same(t, conn, result.Conn)
	require.Equal(t, agentID, result.AgentID)
	require.False(t, result.WasSwitched)
	require.EqualValues(t, 1, validateCalls.Load())

	if result.Release != nil {
		result.Release()
	}
	require.EqualValues(t, 1, releaseCalls.Load())
}

func TestDialWithLazyValidation_SlowDialNoCurrentAgent(t *testing.T) {
	t.Parallel()

	staleAgentID := uuid.New()
	workspaceID := uuid.New()
	dialStarted := make(chan struct{})
	resultCh := make(chan error, 1)

	var dialCalls atomic.Int32
	var validateCalls atomic.Int32

	go func() {
		_, err := dialWithLazyValidation(
			context.Background(),
			staleAgentID,
			workspaceID,
			func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				if id != staleAgentID {
					return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
				}
				dialCalls.Add(1)
				close(dialStarted)
				<-ctx.Done()
				return nil, nil, ctx.Err()
			},
			func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
				if id != workspaceID {
					return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
				}
				<-dialStarted
				validateCalls.Add(1)
				return uuid.Nil, errChatHasNoWorkspaceAgent
			},
			0,
		)
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		require.ErrorIs(t, err, errChatHasNoWorkspaceAgent)
	case <-time.After(testutil.WaitShort):
		t.Fatal("dialWithLazyValidation blocked after validation reported no current agent")
	}

	require.EqualValues(t, 1, dialCalls.Load())
	require.EqualValues(t, 1, validateCalls.Load())
}

func TestDialWithLazyValidation_SlowDialStaleAgent(t *testing.T) {
	t.Parallel()

	t.Run("LateSuccessReleasesStaleConn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		staleAgentID := uuid.New()
		currentAgentID := uuid.New()
		workspaceID := uuid.New()
		staleConn := agentconnmock.NewMockAgentConn(ctrl)
		currentConn := agentconnmock.NewMockAgentConn(ctrl)

		var dialCalls atomic.Int32
		var validateCalls atomic.Int32
		var staleReleaseCalls atomic.Int32
		var currentReleaseCalls atomic.Int32

		result, err := dialWithLazyValidation(
			context.Background(),
			staleAgentID,
			workspaceID,
			func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				dialCalls.Add(1)
				switch id {
				case staleAgentID:
					<-ctx.Done()
					return staleConn, func() {
						staleReleaseCalls.Add(1)
					}, nil
				case currentAgentID:
					return currentConn, func() {
						currentReleaseCalls.Add(1)
					}, nil
				default:
					return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
				}
			},
			func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
				if id != workspaceID {
					return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
				}
				validateCalls.Add(1)
				return currentAgentID, nil
			},
			0,
		)
		require.NoError(t, err)
		require.Same(t, currentConn, result.Conn)
		require.Equal(t, currentAgentID, result.AgentID)
		require.True(t, result.WasSwitched)
		require.Eventually(t, func() bool {
			return dialCalls.Load() == 2
		}, testutil.WaitShort, testutil.IntervalFast)
		require.EqualValues(t, 1, validateCalls.Load())
		require.Eventually(t, func() bool {
			return staleReleaseCalls.Load() == 1
		}, testutil.WaitShort, testutil.IntervalFast)

		if result.Release != nil {
			result.Release()
		}
		require.EqualValues(t, 1, currentReleaseCalls.Load())
	})

	t.Run("CanceledFailureDoesNotReleaseStaleConn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		staleAgentID := uuid.New()
		currentAgentID := uuid.New()
		workspaceID := uuid.New()
		currentConn := agentconnmock.NewMockAgentConn(ctrl)

		var dialCalls atomic.Int32
		var validateCalls atomic.Int32
		var staleReleaseCalls atomic.Int32
		var currentReleaseCalls atomic.Int32

		result, err := dialWithLazyValidation(
			context.Background(),
			staleAgentID,
			workspaceID,
			func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				dialCalls.Add(1)
				switch id {
				case staleAgentID:
					<-ctx.Done()
					return nil, func() {
						staleReleaseCalls.Add(1)
					}, ctx.Err()
				case currentAgentID:
					return currentConn, func() {
						currentReleaseCalls.Add(1)
					}, nil
				default:
					return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
				}
			},
			func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
				if id != workspaceID {
					return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
				}
				validateCalls.Add(1)
				return currentAgentID, nil
			},
			0,
		)
		require.NoError(t, err)
		require.Same(t, currentConn, result.Conn)
		require.Equal(t, currentAgentID, result.AgentID)
		require.True(t, result.WasSwitched)
		require.Eventually(t, func() bool {
			return dialCalls.Load() == 2
		}, testutil.WaitShort, testutil.IntervalFast)
		require.EqualValues(t, 1, validateCalls.Load())
		require.EqualValues(t, 0, staleReleaseCalls.Load())

		if result.Release != nil {
			result.Release()
		}
		require.EqualValues(t, 1, currentReleaseCalls.Load())
	})

	t.Run("SwitchDoesNotBlock", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		staleAgentID := uuid.New()
		currentAgentID := uuid.New()
		workspaceID := uuid.New()
		staleConn := agentconnmock.NewMockAgentConn(ctrl)
		currentConn := agentconnmock.NewMockAgentConn(ctrl)
		staleDialStarted := make(chan struct{})
		allowStaleReturn := make(chan struct{})

		var dialCalls atomic.Int32
		var validateCalls atomic.Int32
		var staleReleaseCalls atomic.Int32
		var currentReleaseCalls atomic.Int32
		var staleReturnReleased atomic.Bool
		releaseStaleReturn := func() {
			if staleReturnReleased.CompareAndSwap(false, true) {
				close(allowStaleReturn)
			}
		}
		defer releaseStaleReturn()

		resultCh := make(chan DialResult, 1)
		errCh := make(chan error, 1)
		go func() {
			result, err := dialWithLazyValidation(
				context.Background(),
				staleAgentID,
				workspaceID,
				func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					dialCalls.Add(1)
					switch id {
					case staleAgentID:
						close(staleDialStarted)
						<-allowStaleReturn
						return staleConn, func() {
							staleReleaseCalls.Add(1)
						}, nil
					case currentAgentID:
						return currentConn, func() {
							currentReleaseCalls.Add(1)
						}, nil
					default:
						return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
					}
				},
				func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
					if id != workspaceID {
						return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
					}
					<-staleDialStarted
					validateCalls.Add(1)
					return currentAgentID, nil
				},
				0,
			)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}()

		var result DialResult
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case result = <-resultCh:
			require.Same(t, currentConn, result.Conn)
			require.Equal(t, currentAgentID, result.AgentID)
			require.True(t, result.WasSwitched)
			releaseStaleReturn()
		case <-time.After(testutil.WaitShort):
			t.Fatal("dialWithLazyValidation blocked on stale dial cleanup")
		}

		require.EqualValues(t, 2, dialCalls.Load())
		require.EqualValues(t, 1, validateCalls.Load())
		require.Eventually(t, func() bool {
			return staleReleaseCalls.Load() == 1
		}, testutil.WaitShort, testutil.IntervalFast)

		if result.Release != nil {
			result.Release()
		}
		require.EqualValues(t, 1, currentReleaseCalls.Load())
	})
}

func TestDialWithLazyValidation_FastFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	staleAgentID := uuid.New()
	currentAgentID := uuid.New()
	workspaceID := uuid.New()
	currentConn := agentconnmock.NewMockAgentConn(ctrl)

	var dialCalls atomic.Int32
	var validateCalls atomic.Int32
	var currentReleaseCalls atomic.Int32

	result, err := dialWithLazyValidation(
		context.Background(),
		staleAgentID,
		workspaceID,
		func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			switch dialCalls.Add(1) {
			case 1:
				if id != staleAgentID {
					return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
				}
				return nil, nil, xerrors.New("dial failed")
			case 2:
				if id != currentAgentID {
					return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
				}
				return currentConn, func() {
					currentReleaseCalls.Add(1)
				}, nil
			default:
				return nil, nil, xerrors.New("unexpected dial call")
			}
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			return currentAgentID, nil
		},
		time.Minute,
	)
	require.NoError(t, err)
	require.Same(t, currentConn, result.Conn)
	require.Equal(t, currentAgentID, result.AgentID)
	require.True(t, result.WasSwitched)
	require.EqualValues(t, 2, dialCalls.Load())
	require.EqualValues(t, 1, validateCalls.Load())

	if result.Release != nil {
		result.Release()
	}
	require.EqualValues(t, 1, currentReleaseCalls.Load())
}

func TestDialWithLazyValidation_FastFailureSameAgent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	conn := agentconnmock.NewMockAgentConn(ctrl)

	var dialCalls atomic.Int32
	var releaseCalls atomic.Int32
	var validateCalls atomic.Int32

	result, err := dialWithLazyValidation(
		context.Background(),
		agentID,
		workspaceID,
		func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			switch dialCalls.Add(1) {
			case 1:
				return nil, nil, xerrors.New("dial failed")
			case 2:
				return conn, func() {
					releaseCalls.Add(1)
				}, nil
			default:
				return nil, nil, xerrors.New("unexpected dial call")
			}
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			return agentID, nil
		},
		time.Minute,
	)
	require.NoError(t, err)
	require.Same(t, conn, result.Conn)
	require.Equal(t, agentID, result.AgentID)
	require.False(t, result.WasSwitched)
	require.EqualValues(t, 2, dialCalls.Load())
	require.EqualValues(t, 1, validateCalls.Load())

	if result.Release != nil {
		result.Release()
	}
	require.EqualValues(t, 1, releaseCalls.Load())
}

func TestDialWithLazyValidation_FastFailureSameAgentRetryFails(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	workspaceID := uuid.New()

	var dialCalls atomic.Int32
	var validateCalls atomic.Int32

	_, err := dialWithLazyValidation(
		context.Background(),
		agentID,
		workspaceID,
		func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			switch dialCalls.Add(1) {
			case 1:
				return nil, nil, xerrors.New("dial failed")
			case 2:
				return nil, nil, xerrors.New("retry failed")
			default:
				return nil, nil, xerrors.New("unexpected dial call")
			}
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			return agentID, nil
		},
		time.Minute,
	)
	require.EqualError(t, err, "dial with lazy validation: retry failed")
	require.EqualValues(t, 2, dialCalls.Load())
	require.EqualValues(t, 1, validateCalls.Load())
}

func TestDialWithLazyValidation_ValidationError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	conn := agentconnmock.NewMockAgentConn(ctrl)
	unblockDial := make(chan struct{})

	var releaseCalls atomic.Int32
	var validateCalls atomic.Int32

	result, err := dialWithLazyValidation(
		context.Background(),
		agentID,
		workspaceID,
		func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			select {
			case <-unblockDial:
				return conn, func() {
					releaseCalls.Add(1)
				}, nil
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			// Validation fails — code should fall back to waiting
			// for the original dial.
			close(unblockDial)
			return uuid.Nil, xerrors.New("db connection reset")
		},
		0,
	)
	require.NoError(t, err)
	require.Same(t, conn, result.Conn)
	require.Equal(t, agentID, result.AgentID)
	require.False(t, result.WasSwitched)
	require.EqualValues(t, 1, validateCalls.Load())

	if result.Release != nil {
		result.Release()
	}
	require.EqualValues(t, 1, releaseCalls.Load())
}

func TestDialWithLazyValidation_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentID := uuid.New()
	workspaceID := uuid.New()

	var validateCalls atomic.Int32

	_, err := dialWithLazyValidation(
		ctx,
		agentID,
		workspaceID,
		func(ctx context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if id != agentID {
				return nil, nil, xerrors.Errorf("unexpected agent ID %q", id)
			}
			<-ctx.Done()
			return nil, nil, ctx.Err()
		},
		func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id != workspaceID {
				return uuid.Nil, xerrors.Errorf("unexpected workspace ID %q", id)
			}
			validateCalls.Add(1)
			cancel()
			return agentID, nil
		},
		0,
	)
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, validateCalls.Load())
}
