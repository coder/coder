package agentsdk_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStartupLogsWriter_Write(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		level      codersdk.LogLevel
		source     codersdk.WorkspaceAgentLogSource
		writes     []string
		want       []agentsdk.Log
		wantErr    bool
		closeFirst bool
	}{
		{
			name:   "single line",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"hello world\n"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
			},
		},
		{
			name:   "multiple lines",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"hello world\n", "goodbye world\n"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "goodbye world",
				},
			},
		},
		{
			name:   "multiple newlines",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"\n\n", "hello world\n\n\n", "goodbye world\n"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "goodbye world",
				},
			},
		},
		{
			name:   "multiple lines with partial",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"hello world\n", "goodbye world"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
			},
		},
		{
			name:       "multiple lines with partial, close flushes",
			ctx:        context.Background(),
			level:      codersdk.LogLevelInfo,
			writes:     []string{"hello world\n", "goodbye world"},
			closeFirst: true,
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "goodbye world",
				},
			},
		},
		{
			name:   "multiple lines with partial in middle",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"hello world\n", "goodbye", " world\n"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "goodbye world",
				},
			},
		},
		{
			name:   "removes carriage return when grouped with newline",
			ctx:    context.Background(),
			level:  codersdk.LogLevelInfo,
			writes: []string{"hello world\r\n", "\r\r\n", "goodbye world\n"},
			want: []agentsdk.Log{
				{
					Level:  codersdk.LogLevelInfo,
					Output: "hello world",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "\r",
				},
				{
					Level:  codersdk.LogLevelInfo,
					Output: "goodbye world",
				},
			},
		},
		{
			name: "cancel context",
			ctx:  canceledCtx,
			writes: []string{
				"hello world\n",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []agentsdk.Log
			send := func(ctx context.Context, log ...agentsdk.Log) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				got = append(got, log...)
				return nil
			}
			w := agentsdk.LogsWriter(tt.ctx, send, uuid.New(), tt.level)
			for _, s := range tt.writes {
				_, err := w.Write([]byte(s))
				if err != nil {
					if tt.wantErr {
						return
					}
					t.Errorf("startupLogsWriter.Write() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			if tt.closeFirst {
				err := w.Close()
				if err != nil {
					t.Errorf("startupLogsWriter.Close() error = %v", err)
					return
				}
			}

			// Compare got and want, but ignore the CreatedAt field.
			for i := range got {
				got[i].CreatedAt = tt.want[i].CreatedAt
			}
			require.Equal(t, tt.want, got)

			err := w.Close()
			if !tt.closeFirst && (err != nil) != tt.wantErr {
				t.Errorf("startupLogsWriter.Close() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

type statusError int

func (s statusError) StatusCode() int {
	return int(s)
}

func (s statusError) Error() string {
	return fmt.Sprintf("status %d", s)
}

func TestStartupLogsSender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sendCount int
		discard   []int
		patchResp func(req agentsdk.PatchLogs) error
	}{
		{
			name:      "single log",
			sendCount: 1,
		},
		{
			name:      "multiple logs",
			sendCount: 995,
		},
		{
			name:      "too large",
			sendCount: 1,
			discard:   []int{1},
			patchResp: func(req agentsdk.PatchLogs) error {
				return statusError(http.StatusRequestEntityTooLarge)
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			defer cancel()

			got := []agentsdk.Log{}
			patchLogs := func(_ context.Context, req agentsdk.PatchLogs) error {
				if tt.patchResp != nil {
					err := tt.patchResp(req)
					if err != nil {
						return err
					}
				}
				got = append(got, req.Logs...)
				return nil
			}

			sendLog, flushAndClose := agentsdk.LogsSender(uuid.New(), patchLogs, slogtest.Make(t, nil).Leveled(slog.LevelDebug))
			defer func() {
				err := flushAndClose(ctx)
				require.NoError(t, err)
			}()

			var want []agentsdk.Log
			for i := 0; i < tt.sendCount; i++ {
				want = append(want, agentsdk.Log{
					CreatedAt: time.Now(),
					Level:     codersdk.LogLevelInfo,
					Output:    fmt.Sprintf("hello world %d", i),
				})
				err := sendLog(ctx, want[len(want)-1])
				require.NoError(t, err)
			}

			err := flushAndClose(ctx)
			require.NoError(t, err)

			for _, di := range tt.discard {
				want = slices.Delete(want, di-1, di)
			}

			require.Equal(t, want, got)
		})
	}

	t.Run("context canceled during send", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		patchLogs := func(_ context.Context, _ agentsdk.PatchLogs) error {
			assert.Fail(t, "should not be called")
			return nil
		}

		sendLog, flushAndClose := agentsdk.LogsSender(uuid.New(), patchLogs, slogtest.Make(t, nil).Leveled(slog.LevelDebug))
		defer func() {
			_ = flushAndClose(ctx)
		}()

		cancel()
		err := sendLog(ctx, agentsdk.Log{
			CreatedAt: time.Now(),
			Level:     codersdk.LogLevelInfo,
			Output:    "hello world",
		})
		require.Error(t, err)

		ctx, cancel = context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		err = flushAndClose(ctx)
		require.NoError(t, err)
	})

	t.Run("context canceled during flush", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		var want, got []agentsdk.Log
		patchLogs := func(_ context.Context, req agentsdk.PatchLogs) error {
			got = append(got, req.Logs...)
			return nil
		}

		sendLog, flushAndClose := agentsdk.LogsSender(uuid.New(), patchLogs, slogtest.Make(t, nil).Leveled(slog.LevelDebug))
		defer func() {
			_ = flushAndClose(ctx)
		}()

		err := sendLog(ctx, agentsdk.Log{
			CreatedAt: time.Now(),
			Level:     codersdk.LogLevelInfo,
			Output:    "hello world",
		})
		require.NoError(t, err)

		cancel()
		err = flushAndClose(ctx)
		require.Error(t, err)

		require.Equal(t, want, got)
	})
}
