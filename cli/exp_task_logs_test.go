package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskLogs(t *testing.T) {
	t.Parallel()

	var (
		clock = time.Date(2025, 8, 26, 12, 34, 56, 0, time.UTC)

		taskID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
		taskName = "task-workspace"

		taskLogs = []codersdk.TaskLogEntry{
			{
				ID:      0,
				Content: "What is 1 + 1?",
				Type:    codersdk.TaskLogTypeInput,
				Time:    clock,
			},
			{
				ID:      1,
				Content: "2",
				Type:    codersdk.TaskLogTypeOutput,
				Time:    clock.Add(1 * time.Second),
			},
		}
	)

	tests := []struct {
		args        []string
		expectLogs  []codersdk.TaskLogEntry
		expectError string
		handler     func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			args:       []string{taskName},
			expectLogs: taskLogs,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/users/me/workspace/%s", taskName):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.TaskLogsResponse{
							Logs: taskLogs,
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:       []string{taskID.String()},
			expectLogs: taskLogs,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.TaskLogsResponse{
							Logs: taskLogs,
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"doesnotexist"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/doesnotexist":
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{uuid.Nil.String()}, // uuid does not exist
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", uuid.Nil.String()):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"err-fetching-logs"},
			expectError: assert.AnError.Error(),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/err-fetching-logs":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/logs", taskID.String()):
						httpapi.InternalServerError(w, assert.AnError)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, ","), func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				srv    = httptest.NewServer(tt.handler(t, ctx))
				client = codersdk.New(testutil.MustURL(t, srv.URL))
				args   = []string{"exp", "task", "logs"}
				stdout strings.Builder
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Stdout = &stdout
			inv.Stderr = &stdout
			clitest.SetupConfig(t, client, root)

			err = inv.WithContext(ctx).Run()
			if tt.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectError)
			}

			if tt.expectLogs != nil {
				lines := strings.Split(stdout.String(), "\n")
				lines = slice.Filter(lines, func(s string) bool {
					return s != ""
				})

				require.Len(t, lines, len(tt.expectLogs))

				for i, line := range lines {
					var log codersdk.TaskLogEntry
					err = json.NewDecoder(strings.NewReader(line)).Decode(&log)
					require.NoError(t, err)

					assert.Equal(t, tt.expectLogs[i], log)
				}
			}
		})
	}
}
