//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/testutil"
)

type openCodeTestTransport func(*http.Request) (*http.Response, error)

func (f openCodeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCodeCoalescesSSEWhileSnapshotBlocked(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	transport := o.server.http.Transport
	var armed atomic.Bool
	var reads atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	o.server.http.Transport = openCodeTestTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/message") && r.URL.Query().Get("cursor") == "" {
			reads.Add(1)
			if armed.Swap(false) {
				close(entered)
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
		}
		return transport.RoundTrip(r)
	})
	f.create(t, o)
	before := reads.Load()
	armed.Store(true)
	answer := openCodeTestText("initial")
	f.mutate(func() {
		f.messages = append(f.messages, map[string]any{"id": openCodeTestID("msg_"), "type": "user", "text": "native input"}, openCodeTestAssistant(answer))
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("snapshot was not started")
	}

	final := strings.Repeat("delta🌏", 1000)
	f.mu.Lock()
	answer["text"] = final
	f.messages = append(f.messages, openCodeTestIdle())
	var stream chan openCodeNativeEvent
	for candidate := range f.streams {
		stream = candidate
	}
	f.mu.Unlock()
	require.NotNil(t, stream)
	id, err := json.Marshal(f.id)
	require.NoError(t, err)
	barrier, err := json.Marshal(openCodeTestID("msg_"))
	require.NoError(t, err)
	for i := range 1001 {
		event := openCodeNativeEvent{ID: uuid.NewString(), Type: "session.message.text.delta", Data: map[string]json.RawMessage{"sessionID": id}}
		if i == 1000 {
			// An unowned cancellation is harmless, but proves the preceding
			// thousand SSE frames were consumed before releasing the snapshot.
			event.Type = "session.inbox.cancelled"
			event.Data["inboxID"] = barrier
		}
		select {
		case stream <- event:
		case <-ctx.Done():
			t.Fatal("SSE reader stopped during burst")
		}
	}
	for {
		o.eventMu.Lock()
		seen := len(o.pending.critical) == 1
		lost, dirty := o.pending.lost, len(o.pending.dirty)
		o.eventMu.Unlock()
		require.False(t, lost)
		if seen {
			require.Equal(t, 1, dirty)
			break
		}
		select {
		case <-o.wake:
		case <-ctx.Done():
			t.Fatal("SSE barrier was not consumed")
		}
	}
	// Replace the wake token consumed while waiting for the ingress barrier.
	select {
	case o.wake <- struct{}{}:
	default:
	}
	releaseOnce.Do(func() { close(release) })
	f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "turn.complete") })
	o.mu.Lock()
	require.NoError(t, o.client.ctx.Err())
	o.mu.Unlock()
	var text strings.Builder
	for _, event := range f.host.all() {
		require.NotEqual(t, "overflow", event.Type)
		if event.Type == "text" {
			_, _ = text.WriteString(event.Data["text"].(string))
		}
	}
	require.Equal(t, final, text.String())
	require.LessOrEqual(t, reads.Load()-before, int32(2), "one blocked read and one coalesced follow-up")
}

func TestOpenCodeCriticalIngressOrderAndBound(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	o.eventSessions[f.id] = true
	o.pending.generation = 7
	id, err := json.Marshal(f.id)
	require.NoError(t, err)
	event := func(kind string) openCodeNativeEvent {
		return openCodeNativeEvent{Type: kind, Data: map[string]json.RawMessage{"sessionID": id}}
	}
	kinds := []string{"session.inbox.cancelled", "session.deleted", "session.revert.committed"}
	for _, kind := range kinds {
		for range 1000 {
			require.True(t, o.enqueue(7, event("session.message.text.delta")))
		}
		require.True(t, o.enqueue(7, event(kind)))
	}
	require.True(t, o.enqueue(6, event("session.deleted")), "stale generations must be ignored")
	pending := o.drain()
	require.Equal(t, map[string]bool{f.id: true}, pending.dirty)
	require.Len(t, pending.critical, len(kinds))
	for i, kind := range kinds {
		require.Equal(t, kind, pending.critical[i].Type)
	}
	for range 128 {
		require.True(t, o.enqueue(7, event("session.deleted")))
	}
	require.False(t, o.enqueue(7, event("session.deleted")))
	pending = o.drain()
	require.True(t, pending.lost, "critical edges may not be silently discarded")
	require.Len(t, pending.critical, 128)
}

func TestOpenCodeLostPromptResponseReconcilesInbox(t *testing.T) {
	t.Parallel()
	for _, mismatch := range []string{"", "text", "session", "type", "missing-payload"} {
		t.Run(mismatch, func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			s := f.create(t, o)
			f.host.automation()
			f.mu.Lock()
			f.losePrompt = true
			f.mu.Unlock()
			prompt := Prompt{ID: uuid.NewString(), Text: "queued despite a lost response"}
			require.Error(t, o.Prompt(t.Context(), s, prompt))
			require.NoError(t, o.Close())
			saved, err := f.host.Session(s.ID)
			require.NoError(t, err)
			receipts, err := readOpenCodeReceipts(saved.State)
			require.NoError(t, err)
			require.Equal(t, "submitting", receipts.Requests[prompt.ID].Status)
			nativeID := receipts.Requests[prompt.ID].NativeID
			f.mu.Lock()
			switch mismatch {
			case "text":
				f.inbox[0]["payload"] = map[string]any{"text": "not our request"}
			case "session":
				f.inbox[0]["sessionID"] = openCodeTestID("ses_")
			case "type":
				f.inbox[0]["type"] = "synthetic"
			case "missing-payload":
				delete(f.inbox[0], "payload")
			}
			f.mu.Unlock()
			restored := f.backend(t)
			err = restored.Resume(t.Context(), saved)
			if mismatch != "" {
				require.Error(t, err)
				require.Error(t, restored.Cancel(t.Context(), saved, prompt.ID))
				require.Len(t, f.mutations(), 2)
				return
			}
			require.NoError(t, err)
			saved, err = f.host.Session(s.ID)
			require.NoError(t, err)
			receipts, err = readOpenCodeReceipts(saved.State)
			require.NoError(t, err)
			require.Equal(t, "queued", receipts.Requests[prompt.ID].Status)
			require.NoError(t, restored.Prompt(t.Context(), saved, prompt))
			require.NoError(t, restored.Cancel(t.Context(), saved, prompt.ID))
			f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "prompt.rejected") })
			calls := f.mutations()
			require.Len(t, calls, 3)
			require.Equal(t, "DELETE", calls[2].method)
			require.Equal(t, "/api/session/"+f.id+"/inbox/"+nativeID, calls[2].path)
		})
	}
}

func TestOpenCodeCriticalEdgesApplyInArrivalOrder(t *testing.T) {
	t.Parallel()
	for _, cancelFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel-first", false: "delete-first"}[cancelFirst], func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			s := f.create(t, o)
			f.host.automation()
			prompt := Prompt{ID: uuid.NewString(), Text: "owned queued input"}
			require.NoError(t, o.Prompt(t.Context(), s, prompt))
			id, err := json.Marshal(f.id)
			require.NoError(t, err)
			inboxID, err := json.Marshal(f.mutations()[1].body["id"])
			require.NoError(t, err)
			func() {
				o.mu.Lock()
				defer o.mu.Unlock()
				types := []string{"session.deleted", "session.inbox.cancelled"}
				if cancelFirst {
					types[0], types[1] = types[1], types[0]
				}
				for _, kind := range types {
					require.True(t, o.enqueue(o.generation, openCodeNativeEvent{Type: kind, Data: map[string]json.RawMessage{"sessionID": id, "inboxID": inboxID}}))
				}
				for range 1000 {
					require.True(t, o.enqueue(o.generation, openCodeNativeEvent{Type: "session.message.text.delta", Data: map[string]json.RawMessage{"sessionID": id}}))
				}
			}()
			f.host.wait(t, func(events []Event) bool { return openCodeHasEvent(events, "overflow") })
			require.Equal(t, cancelFirst, openCodeHasEvent(f.host.all(), "prompt.rejected"))
		})
	}
}

func TestOpenCodeStartingRecoveryRequiresVerifiedReadiness(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"password", "listener", "unrecorded", "locked"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			d := f.descriptor
			d.Status = "starting"
			switch kind {
			case "password":
				d.Password = strings.Repeat("c", 64)
			case "listener":
				process := o.server.process
				o.server.process = func(pid int) (*openCodeProcess, error) {
					p, err := process(pid)
					p.Ports = nil
					return p, err
				}
			case "unrecorded":
				d.PID, d.StartTime = 0, ""
			case "locked":
				unlock, err := o.server.lock()
				require.NoError(t, err)
				defer unlock()
			}
			require.NoError(t, o.server.writeDescriptor(d))
			o.server.version = func(context.Context) (string, error) {
				t.Error("an unverified live server must not be replaced")
				return "", errOpenCodeOwnership
			}
			_, err := o.server.ensure(t.Context())
			require.Error(t, err)
			require.Error(t, o.server.shutdown(t.Context()))
			saved, err := o.server.readDescriptor()
			require.NoError(t, err)
			require.Equal(t, d, *saved)
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Zero(t, f.signals)
		})
	}
}

func TestOpenCodeLifecycleLockRetainsInode(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	unlock, err := o.server.lock()
	require.NoError(t, err)
	defer unlock()
	before, err := os.Stat(f.directory + "/opencode-server.lock")
	require.NoError(t, err)
	// Retain the same open file description as a child between fork and exec.
	// CLOEXEC keeps the test's duplicate out of unrelated helper processes.
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	duplicate := -1
	for _, entry := range entries {
		info, err := os.Stat(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil || !os.SameFile(before, info) {
			continue
		}
		fd, err := strconv.ParseUint(entry.Name(), 10, 32)
		require.NoError(t, err)
		duplicate, err = unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
		require.NoError(t, err)
		break
	}
	require.NotEqual(t, -1, duplicate, "held lifecycle lock descriptor was not found")
	defer func() { _ = unix.Close(duplicate) }()
	_, err = o.server.lock()
	require.Error(t, err)
	unlock()
	unlockNext, err := o.server.lock()
	require.NoError(t, err)
	defer unlockNext()
	after, err := os.Stat(f.directory + "/opencode-server.lock")
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	unlock() // Releasing the old lock twice must not release its successor.
	_, err = o.server.lock()
	require.Error(t, err)
}
