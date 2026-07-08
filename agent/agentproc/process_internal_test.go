package agentproc

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

func TestReapFreesClientTokenIndex(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})
	mClock := quartz.NewMock(t)
	m.clock = mClock

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-1",
	}
	proc, attached, err := m.start(req, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	<-proc.done

	// The token keeps attaching to the exited process until the
	// reap age passes, so a retried start still sees the result.
	again, attached, err := m.start(req, "chat-1")
	require.NoError(t, err)
	require.True(t, attached)
	require.Equal(t, proc.id, again.id)

	mClock.Advance(exitedProcessReapAge + time.Minute)

	// The sweep on start reaps the exited process, frees its
	// token index entry, and the same token starts fresh.
	fresh, attached, err := m.start(req, "chat-1")
	require.NoError(t, err)
	require.False(t, attached)
	require.NotEqual(t, proc.id, fresh.id)

	m.mu.Lock()
	_, tracked := m.procs[proc.id]
	tokenCount := len(m.tokens)
	m.mu.Unlock()
	require.False(t, tracked)
	require.Equal(t, 1, tokenCount)
	<-fresh.done
}

func TestConcurrentSameTokenStartsOnce(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	m := newManager(logger, agentexec.DefaultExecer, nil, nil, nil, nil)
	t.Cleanup(func() {
		_ = m.Close()
	})

	req := workspacesdk.StartProcessRequest{
		Command:     "echo hello",
		ClientToken: "tok-race",
	}

	const workers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		ids      = make(map[string]struct{})
		attaches int
		errs     []error
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proc, attached, err := m.start(req, "chat-1")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids[proc.id] = struct{}{}
			if attached {
				attaches++
			}
		}()
	}
	wg.Wait()
	require.Empty(t, errs)

	// Every concurrent start with the same token must resolve to
	// the same single process, with exactly one actual spawn.
	require.Len(t, ids, 1)
	require.Equal(t, workers-1, attaches)

	m.mu.Lock()
	procCount := len(m.procs)
	m.mu.Unlock()
	require.Equal(t, 1, procCount)
}
