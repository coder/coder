package agentfiles_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// longRunning is how long the agent has been running in tests that need
// every tool call to be provably new to the agent.
const longRunning = time.Hour

// toolCallDir holds the files the tool call tests use. It is absolute on
// every platform, as the file handlers require.
var toolCallDir = filepath.Join(os.TempDir(), "work")

// toolCallFile is the file every tool call test edits or writes. It
// starts with the content "one\n".
var toolCallFile = filepath.Join(toolCallDir, "file.txt")

// fileTool sends one file tool's request for toolCallFile.
type fileTool struct {
	// route is the tool's route, which its cancel route extends.
	route string
	// sendContext posts the tool's request with ctx. other selects a
	// different input for the same file.
	sendContext func(ctx context.Context, t *testing.T, handler http.Handler, other bool, headers http.Header) *httptest.ResponseRecorder
}

// send posts the tool's request. other selects a different input for the
// same file.
func (tool fileTool) send(t *testing.T, handler http.Handler, other bool, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()

	return tool.sendContext(testutil.Context(t, testutil.WaitLong), t, handler, other, headers)
}

var fileTools = []fileTool{
	{
		route: "edit-files",
		sendContext: func(ctx context.Context, t *testing.T, handler http.Handler, other bool, headers http.Header) *httptest.ResponseRecorder {
			newText := "one two"
			if other {
				newText = "one three"
			}
			return postEditFiles(ctx, t, handler, workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{
					Path:  toolCallFile,
					Edits: []workspacesdk.FileEdit{{OldText: "one", NewText: newText}},
				}},
				IncludeDiff: true,
			}, headers)
		},
	},
	{
		route: "write-file",
		sendContext: func(ctx context.Context, t *testing.T, handler http.Handler, other bool, headers http.Header) *httptest.ResponseRecorder {
			content := "one two\n"
			if other {
				content = "three\n"
			}
			return postWriteFile(ctx, handler, toolCallFile, content, headers)
		},
	},
}

func TestFileToolCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// uptime is how long the agent has been running. Zero means
		// longRunning.
		uptime time.Duration
		// setup sends earlier requests for the chat and returns the
		// response the tested request must replay, if any.
		setup     func(t *testing.T, tool fileTool, handler http.Handler, chatID uuid.UUID) *httptest.ResponseRecorder
		messageID int64
		age       time.Duration
		wantCode  int
		wantError workspacesdk.ToolCallErrorCode
		// wantWrites is how many times the file was written in total.
		wantWrites int
	}{
		{
			name:       "New",
			messageID:  1,
			wantCode:   http.StatusOK,
			wantWrites: 1,
		},
		{
			name: "RepeatedSameInput",
			setup: func(t *testing.T, tool fileTool, handler http.Handler, chatID uuid.UUID) *httptest.ResponseRecorder {
				w := tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				return w
			},
			messageID:  1,
			wantCode:   http.StatusOK,
			wantWrites: 1,
		},
		{
			name: "InputMismatch",
			setup: func(t *testing.T, tool fileTool, handler http.Handler, chatID uuid.UUID) *httptest.ResponseRecorder {
				w := tool.send(t, handler, true, toolCallHeaders(chatID, 1, "call", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				return nil
			},
			messageID:  1,
			wantCode:   http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorInputMismatch,
			wantWrites: 1,
		},
		{
			name: "StaleMessage",
			setup: func(t *testing.T, tool fileTool, handler http.Handler, chatID uuid.UUID) *httptest.ResponseRecorder {
				w := tool.send(t, handler, true, toolCallHeaders(chatID, 2, "other", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				return nil
			},
			messageID:  1,
			wantCode:   http.StatusConflict,
			wantError:  workspacesdk.ToolCallErrorStale,
			wantWrites: 1,
		},
		{
			name:      "AgentStartedAfterToolCall",
			uptime:    10 * time.Second,
			messageID: 1,
			age:       time.Minute,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorAgentStartedAfterToolCall,
		},
		{
			name: "CanceledBeforeStart",
			setup: func(t *testing.T, tool fileTool, handler http.Handler, chatID uuid.UUID) *httptest.ResponseRecorder {
				resp := requireCancelFile(t, handler, tool.route, chatID, 1, "call", 0)
				require.False(t, resp.Started)
				return nil
			},
			messageID: 1,
			wantCode:  http.StatusConflict,
			wantError: workspacesdk.ToolCallErrorCanceled,
		},
	}
	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					var writes atomic.Int64
					handler, _, fs := newToolCallTestAPI(t, cmp.Or(tt.uptime, longRunning), countWrites(&writes))
					chatID := uuid.New()
					var first *httptest.ResponseRecorder
					if tt.setup != nil {
						first = tt.setup(t, tool, handler, chatID)
					}
					before := readToolCallFile(t, fs)

					w := tool.send(t, handler, false, toolCallHeaders(chatID, tt.messageID, "call", tt.age))
					require.Equal(t, tt.wantCode, w.Code, w.Body.String())
					if tt.wantCode == http.StatusOK {
						if first != nil {
							assert.Equal(t, first.Body.String(), w.Body.String(), "a repeated request gets the recorded response")
							assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
							assert.Equal(t, first.Header().Get("Content-Type"), w.Header().Get("Content-Type"))
							assert.Equal(t, before, readToolCallFile(t, fs))
						}
						assert.Equal(t, "one two\n", readToolCallFile(t, fs))
					} else {
						require.Equal(t, tt.wantError, decodeToolCallError(t, w).Code)
						assert.Equal(t, before, readToolCallFile(t, fs), "a refused request does not touch the file")
					}
					assert.EqualValues(t, tt.wantWrites, writes.Load())
				})
			}
		})
	}
}

// TestFileToolCallRecordedError verifies that a failed edit or write is
// recorded like a successful one: a repeated request gets the same error
// even after a fresh request would succeed.
func TestFileToolCallRecordedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		route string
		// fail makes the tool's request fail, and fix makes it succeed.
		fail, fix func(t *testing.T, fs afero.Fs)
	}{
		{
			route: "edit-files",
			fail: func(t *testing.T, fs afero.Fs) {
				require.NoError(t, afero.WriteFile(fs, toolCallFile, []byte("zero\n"), 0o644))
			},
			fix: func(t *testing.T, fs afero.Fs) {
				require.NoError(t, afero.WriteFile(fs, toolCallFile, []byte("one\n"), 0o644))
			},
		},
		{
			// A directory at the path makes the write fail.
			route: "write-file",
			fail: func(t *testing.T, fs afero.Fs) {
				require.NoError(t, fs.Remove(toolCallFile))
				require.NoError(t, fs.Mkdir(toolCallFile, 0o755))
			},
			fix: func(t *testing.T, fs afero.Fs) {
				require.NoError(t, fs.Remove(toolCallFile))
				require.NoError(t, afero.WriteFile(fs, toolCallFile, []byte("one\n"), 0o644))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			t.Parallel()

			var send func(t *testing.T, handler http.Handler, other bool, headers http.Header) *httptest.ResponseRecorder
			for _, tool := range fileTools {
				if tool.route == tt.route {
					send = tool.send
				}
			}
			var writes atomic.Int64
			handler, _, fs := newToolCallTestAPI(t, longRunning, countWrites(&writes))
			tt.fail(t, fs)
			chatID := uuid.New()

			first := send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
			require.Equal(t, http.StatusBadRequest, first.Code, first.Body.String())

			tt.fix(t, fs)
			again := send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
			require.Equal(t, http.StatusBadRequest, again.Code, again.Body.String())
			assert.Equal(t, first.Body.String(), again.Body.String())
			assert.Equal(t, "one\n", readToolCallFile(t, fs))
			assert.Zero(t, writes.Load())
		})
	}
}

// TestFileToolCallWaiterContextEnds ends a repeated request's context while
// the first request applies the edit. The waiter returns without applying
// it, and the edit is applied once.
func TestFileToolCallWaiterContextEnds(t *testing.T) {
	t.Parallel()

	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			reached := make(chan struct{}, 1)
			release := make(chan struct{})
			var writes atomic.Int64
			handler, _, fs := newToolCallTestAPI(t, longRunning, func(call, file string) error {
				if call == "rename" && file == toolCallFile {
					writes.Add(1)
					reached <- struct{}{}
					<-release
				}
				return nil
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			chatID := uuid.New()

			applied := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				applied <- tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
			}()
			testutil.RequireReceive(ctx, t, reached)

			// The waiter's context has ended, so it returns instead of
			// waiting for the edit in progress.
			waiterCtx, cancel := context.WithCancel(ctx)
			cancel()
			waiter := tool.sendContext(waiterCtx, t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
			assert.Equal(t, http.StatusInternalServerError, waiter.Code, waiter.Body.String())

			close(release)
			w := testutil.RequireReceive(ctx, t, applied)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.EqualValues(t, 1, writes.Load())
			assert.Equal(t, "one two\n", readToolCallFile(t, fs))
		})
	}
}

func TestFileToolCallConcurrent(t *testing.T) {
	t.Parallel()

	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			var writes atomic.Int64
			handler, _, fs := newToolCallTestAPI(t, longRunning, countWrites(&writes))
			chatID := uuid.New()

			const callers = 8
			bodies := make([]string, callers)
			var wg sync.WaitGroup
			for i := range callers {
				wg.Go(func() {
					w := tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
					assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
					bodies[i] = w.Body.String()
				})
			}
			wg.Wait()

			for _, body := range bodies {
				require.Equal(t, bodies[0], body)
			}
			assert.EqualValues(t, 1, writes.Load())
			assert.Equal(t, "one two\n", readToolCallFile(t, fs))
		})
	}
}

func TestFileToolCallWithoutToolCallUnchanged(t *testing.T) {
	t.Parallel()

	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			var writes atomic.Int64
			handler, _, _ := newToolCallTestAPI(t, longRunning, countWrites(&writes))
			headers := http.Header{workspacesdk.CoderChatIDHeader: {uuid.New().String()}}

			for range 2 {
				w := tool.send(t, handler, false, headers)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			}
			assert.EqualValues(t, 2, writes.Load(), "requests without tool call headers are independent")
		})
	}
}

func TestFileToolCallBadRequest(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	id := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
	withChat := func(h http.Header) http.Header {
		h.Set(workspacesdk.CoderChatIDHeader, chatID.String())
		return h
	}
	toolCallOnly := func() http.Header {
		h := http.Header{}
		workspacesdk.ToolCall{MessageID: 1, ID: "call"}.SetHeaders(h)
		return h
	}
	partial := http.Header{workspacesdk.CoderToolCallMessageIDHeader: {"1"}}
	malformedAge := toolCallOnly()
	malformedAge.Set(workspacesdk.CoderToolCallAgeMsHeader, "-1")

	tests := []struct {
		name    string
		cancel  bool
		id      string
		headers http.Header
	}{
		{name: "WithoutChat", headers: toolCallOnly()},
		{name: "PartialHeaders", headers: withChat(partial.Clone())},
		{name: "MalformedAge", headers: withChat(malformedAge.Clone())},
		{name: "CancelWithoutChat", cancel: true, id: id, headers: toolCallOnly()},
		{name: "CancelWithoutToolCall", cancel: true, id: id, headers: withChat(http.Header{})},
		{name: "CancelPartialHeaders", cancel: true, id: id, headers: withChat(partial.Clone())},
		{name: "CancelMalformedAge", cancel: true, id: id, headers: withChat(malformedAge.Clone())},
		{name: "CancelWrongID", cancel: true, id: uuid.New().String(), headers: toolCallHeaders(chatID, 1, "call", 0)},
	}
	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					var writes atomic.Int64
					handler, _, _ := newToolCallTestAPI(t, longRunning, countWrites(&writes))
					var w *httptest.ResponseRecorder
					if tt.cancel {
						w = postCancelFile(t, handler, tool.route, tt.id, tt.headers)
					} else {
						w = tool.send(t, handler, false, tt.headers)
					}
					require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
					assert.Zero(t, writes.Load())
					// Nothing was recorded, so a valid request still applies.
					w = tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					assert.EqualValues(t, 1, writes.Load())
				})
			}
		})
	}
}

func TestCancelFileToolCall(t *testing.T) {
	t.Parallel()

	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			t.Run("Applied", func(t *testing.T) {
				t.Parallel()

				handler, _, _ := newToolCallTestAPI(t, longRunning, nil)
				chatID := uuid.New()
				w := tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())

				resp := requireCancelFile(t, handler, tool.route, chatID, 1, "call", 0)
				assert.True(t, resp.Started)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.JSONEq(t, w.Body.String(), string(resp.Body))
			})

			t.Run("InProgress", func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					// Blocking the rename holds the edit after its record is
					// inserted and before its response is recorded.
					reached := make(chan struct{}, 1)
					release := make(chan struct{})
					handler, _, fs := newToolCallTestAPI(t, longRunning, func(call, file string) error {
						if call == "rename" && file == toolCallFile {
							reached <- struct{}{}
							<-release
						}
						return nil
					})
					chatID := uuid.New()

					applied := make(chan *httptest.ResponseRecorder, 1)
					go func() {
						applied <- tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
					}()
					<-reached

					canceled := make(chan *httptest.ResponseRecorder, 1)
					go func() {
						id := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
						canceled <- postCancelFile(t, handler, tool.route, id, toolCallHeaders(chatID, 1, "call", 0))
					}()
					// Every goroutine is now durably blocked: the edit on
					// release, the cancel inside Records.Cancel.
					synctest.Wait()
					require.Empty(t, canceled, "cancel must wait for the edit in progress")
					close(release)

					w := <-applied
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					cw := <-canceled
					require.Equal(t, http.StatusOK, cw.Code, cw.Body.String())
					var resp workspacesdk.CancelFileToolCallResponse
					require.NoError(t, json.NewDecoder(cw.Body).Decode(&resp))
					assert.True(t, resp.Started)
					assert.Equal(t, http.StatusOK, resp.StatusCode)
					assert.JSONEq(t, w.Body.String(), string(resp.Body))
					assert.Equal(t, "one two\n", readToolCallFile(t, fs), "an edit in progress finishes")
				})
			})

			t.Run("ContextEndsWhileWaiting", func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					reached := make(chan struct{}, 1)
					release := make(chan struct{})
					handler, _, fs := newToolCallTestAPI(t, longRunning, func(call, file string) error {
						if call == "rename" && file == toolCallFile {
							reached <- struct{}{}
							<-release
						}
						return nil
					})
					chatID := uuid.New()

					applied := make(chan *httptest.ResponseRecorder, 1)
					go func() {
						applied <- tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
					}()
					<-reached

					cancelCtx, cancel := context.WithCancel(t.Context())
					canceled := make(chan *httptest.ResponseRecorder, 1)
					go func() {
						id := workspacesdk.ToolCallUUID(chatID, 1, "call").String()
						canceled <- postCancelFileContext(cancelCtx, handler, tool.route, id, toolCallHeaders(chatID, 1, "call", 0))
					}()
					synctest.Wait()
					cancel()

					// The client is gone, so the handler writes nothing.
					cw := <-canceled
					assert.Empty(t, cw.Body.String())
					assert.Empty(t, cw.Header().Get("Content-Type"))

					close(release)
					w := <-applied
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					assert.Equal(t, "one two\n", readToolCallFile(t, fs), "the edit finishes")
				})
			})

			t.Run("RecordedError", func(t *testing.T) {
				t.Parallel()

				handler, _, fs := newToolCallTestAPI(t, longRunning, nil)
				dir := filepath.Join(toolCallDir, "dir")
				require.NoError(t, fs.Mkdir(dir, 0o755))
				chatID := uuid.New()
				var w *httptest.ResponseRecorder
				if tool.route == "write-file" {
					w = postWriteFile(testutil.Context(t, testutil.WaitLong), handler, dir, "content", toolCallHeaders(chatID, 1, "call", 0))
				} else {
					w = postEditFiles(testutil.Context(t, testutil.WaitLong), t, handler, workspacesdk.FileEditRequest{
						Files: []workspacesdk.FileEdits{{Path: dir, Edits: []workspacesdk.FileEdit{{OldText: "a", NewText: "b"}}}},
					}, toolCallHeaders(chatID, 1, "call", 0))
				}
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

				resp := requireCancelFile(t, handler, tool.route, chatID, 1, "call", 0)
				assert.True(t, resp.Started)
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.JSONEq(t, w.Body.String(), string(resp.Body))
				var body codersdk.Response
				require.NoError(t, json.Unmarshal(resp.Body, &body))
				assert.NotEmpty(t, body.Message)
			})

			t.Run("NeverReceived", func(t *testing.T) {
				t.Parallel()

				var writes atomic.Int64
				handler, _, _ := newToolCallTestAPI(t, longRunning, countWrites(&writes))
				chatID := uuid.New()
				for range 2 {
					resp := requireCancelFile(t, handler, tool.route, chatID, 1, "call", 0)
					assert.Equal(t, workspacesdk.CancelFileToolCallResponse{}, resp)
				}
				w := tool.send(t, handler, false, toolCallHeaders(chatID, 1, "call", 0))
				require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
				assert.Equal(t, workspacesdk.ToolCallErrorCanceled, decodeToolCallError(t, w).Code)
				assert.Zero(t, writes.Load())
			})

			t.Run("AgentStartedAfterToolCall", func(t *testing.T) {
				t.Parallel()

				handler, _, _ := newToolCallTestAPI(t, 10*time.Second, nil)
				chatID := uuid.New()
				w := postCancelFile(t, handler, tool.route, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", time.Minute))
				require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
				require.Equal(t, workspacesdk.ToolCallErrorAgentStartedAfterToolCall, decodeToolCallError(t, w).Code)
			})

			t.Run("StaleMessage", func(t *testing.T) {
				t.Parallel()

				handler, _, _ := newToolCallTestAPI(t, longRunning, nil)
				chatID := uuid.New()
				w := tool.send(t, handler, false, toolCallHeaders(chatID, 2, "other", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				w = postCancelFile(t, handler, tool.route, workspacesdk.ToolCallUUID(chatID, 1, "call").String(), toolCallHeaders(chatID, 1, "call", 0))
				require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
				require.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
			})

			t.Run("OtherChat", func(t *testing.T) {
				t.Parallel()

				handler, _, _ := newToolCallTestAPI(t, longRunning, nil)
				chatA := uuid.New()
				chatB := uuid.New()
				w := tool.send(t, handler, false, toolCallHeaders(chatA, 1, "call", 0))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())

				// Chat B cannot address chat A's tool call UUID.
				w = postCancelFile(t, handler, tool.route, workspacesdk.ToolCallUUID(chatA, 1, "call").String(), toolCallHeaders(chatB, 1, "call", 0))
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				// The same tool call identifiers in chat B are a different tool call.
				resp := requireCancelFile(t, handler, tool.route, chatB, 1, "call", 0)
				assert.False(t, resp.Started)
			})
		})
	}
}

// newToolCallTestAPI returns a handler whose agent has been running for
// uptime on the returned mock clock, and its file system, which holds
// toolCallFile. intercept, if set, sees every file system call.
func newToolCallTestAPI(t *testing.T, uptime time.Duration, intercept func(call, file string) error) (http.Handler, *quartz.Mock, afero.Fs) {
	t.Helper()

	if intercept == nil {
		intercept = func(string, string) error { return nil }
	}
	fs := newTestFs(afero.NewMemMapFs(), intercept)
	require.NoError(t, afero.WriteFile(fs.Fs, toolCallFile, []byte("one\n"), 0o644))

	clock := quartz.NewMock(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	api := agentfiles.NewAPI(logger, fs, nil, agentfiles.WithToolCallChats(agenttoolcall.NewChats(clock)))
	clock.Advance(uptime).MustWait(testutil.Context(t, testutil.WaitShort))
	return agentchat.Middleware(api.Routes()), clock, fs
}

// countWrites returns a file system intercept that counts completed
// writes of toolCallFile. Edits and writes replace the file by renaming
// a temporary file onto it.
func countWrites(writes *atomic.Int64) func(call, file string) error {
	return func(call, file string) error {
		if call == "rename" && file == toolCallFile {
			writes.Add(1)
		}
		return nil
	}
}

func readToolCallFile(t *testing.T, fs afero.Fs) string {
	t.Helper()

	data, err := afero.ReadFile(fs, toolCallFile)
	require.NoError(t, err)
	return string(data)
}

func toolCallHeaders(chatID uuid.UUID, messageID int64, toolCallID string, age time.Duration) http.Header {
	h := http.Header{workspacesdk.CoderChatIDHeader: {chatID.String()}}
	workspacesdk.ToolCall{MessageID: messageID, ID: toolCallID, Age: age}.SetHeaders(h)
	return h
}

func postEditFiles(ctx context.Context, t *testing.T, handler http.Handler, req workspacesdk.FileEditRequest, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)
	return serve(ctx, handler, "/edit-files", bytes.NewReader(body), headers)
}

func postWriteFile(ctx context.Context, handler http.Handler, path, content string, headers http.Header) *httptest.ResponseRecorder {
	return serve(ctx, handler, "/write-file?"+url.Values{"path": {path}}.Encode(), strings.NewReader(content), headers)
}

func postCancelFile(t *testing.T, handler http.Handler, route, id string, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()

	return postCancelFileContext(testutil.Context(t, testutil.WaitLong), handler, route, id, headers)
}

func postCancelFileContext(ctx context.Context, handler http.Handler, route, id string, headers http.Header) *httptest.ResponseRecorder {
	return serve(ctx, handler, fmt.Sprintf("/%s/%s/cancel", route, id), bytes.NewReader(nil), headers)
}

func serve(ctx context.Context, handler http.Handler, target string, body io.Reader, headers http.Header) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, target, body)
	for k, v := range headers {
		r.Header[k] = v
	}
	handler.ServeHTTP(w, r)
	return w
}

// requireCancelFile cancels the tool call's edit or write and requires a
// 200.
func requireCancelFile(t *testing.T, handler http.Handler, route string, chatID uuid.UUID, messageID int64, toolCallID string, age time.Duration) workspacesdk.CancelFileToolCallResponse {
	t.Helper()

	id := workspacesdk.ToolCallUUID(chatID, messageID, toolCallID).String()
	w := postCancelFile(t, handler, route, id, toolCallHeaders(chatID, messageID, toolCallID, age))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.CancelFileToolCallResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func decodeToolCallError(t *testing.T, w *httptest.ResponseRecorder) workspacesdk.ToolCallError {
	t.Helper()

	var resp workspacesdk.ToolCallError
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// TestFileToolCallStaleAfterProcessToolCall covers the process and file
// APIs of one agent sharing each chat's latest message ID: a request for
// a newer message through either makes older tool calls stale on both.
func TestFileToolCallStaleAfterProcessToolCall(t *testing.T) {
	t.Parallel()

	for _, tool := range fileTools {
		t.Run(tool.route, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			chats := agenttoolcall.NewChats(clock)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			var writes atomic.Int64
			fs := newTestFs(afero.NewMemMapFs(), countWrites(&writes))
			require.NoError(t, afero.WriteFile(fs.Fs, toolCallFile, []byte("one\n"), 0o644))
			files := agentchat.Middleware(agentfiles.NewAPI(logger, fs, nil, agentfiles.WithToolCallChats(chats)).Routes())
			procAPI := agentproc.NewAPI(logger, agentexec.DefaultExecer, nil, nil, nil, nil, nil, agentproc.WithClock(clock), agentproc.WithToolCallChats(chats))
			t.Cleanup(func() { _ = procAPI.Close() })
			processes := agentchat.Middleware(procAPI.Routes())
			clock.Advance(longRunning).MustWait(testutil.Context(t, testutil.WaitShort))
			chatID := uuid.New()

			// A cancel for a process tool call in message 2 records it
			// without starting a process.
			processCall := toolCallHeaders(chatID, 2, "process", 0)
			w := serve(testutil.Context(t, testutil.WaitLong), processes, "/"+workspacesdk.ToolCallUUID(chatID, 2, "process").String()+"/cancel", bytes.NewReader(nil), processCall)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			w = tool.send(t, files, false, toolCallHeaders(chatID, 1, "file", 0))
			require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
			assert.Zero(t, writes.Load())

			// The reverse: a file tool call in message 3 makes message 2
			// stale for processes.
			w = tool.send(t, files, false, toolCallHeaders(chatID, 3, "file", 0))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			w = serve(testutil.Context(t, testutil.WaitLong), processes, "/"+workspacesdk.ToolCallUUID(chatID, 2, "process").String()+"/cancel", bytes.NewReader(nil), processCall)
			require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			assert.Equal(t, workspacesdk.ToolCallErrorStale, decodeToolCallError(t, w).Code)
		})
	}
}
