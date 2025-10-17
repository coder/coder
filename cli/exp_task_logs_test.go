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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskLogs(t *testing.T) {
	t.Parallel()

	var (
		clock = time.Date(2025, 8, 26, 12, 34, 56, 0, time.UTC)

		taskID       = uuid.MustParse("11111111-1111-1111-1111-111111111111")
		taskName     = "task-workspace"
		agentID      = uuid.MustParse("22222222-2222-2222-2222-222222222222")
		sidebarAppID = uuid.MustParse("33333333-3333-3333-3333-333333333333")

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

	// Helper to create a workspace response with AI task configuration
	makeTaskWorkspace := func() codersdk.Workspace {
		hasAITask := true
		return codersdk.Workspace{
			ID:   taskID,
			Name: taskName,
			LatestBuild: codersdk.WorkspaceBuild{
				ID:                 uuid.New(),
				HasAITask:          &hasAITask,
				AITaskSidebarAppID: &sidebarAppID,
				Resources: []codersdk.WorkspaceResource{
					{
						Agents: []codersdk.WorkspaceAgent{
							{
								ID: agentID,
								Apps: []codersdk.WorkspaceApp{
									{
										ID:     sidebarAppID,
										URL:    "http://localhost:8080",
										Health: codersdk.WorkspaceAppHealthHealthy,
									},
								},
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name                 string
		args                 []string
		expectTable          string
		expectLogs           []codersdk.TaskLogEntry
		expectError          string
		expectStderrContains string
		handler              func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			name:       "task name with json output",
			args:       []string{taskName, "--output", "json"},
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
			name:       "task ID with json output",
			args:       []string{taskID.String(), "--output", "json"},
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
			name: "task ID with table output",
			args: []string{taskID.String()},
			expectTable: `
TYPE    CONTENT
input   What is 1 + 1?
output  2`,
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
			name:        "non-existent task name",
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
			name:        "non-existent task UUID",
			args:        []string{uuid.Nil.String()},
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
			name:        "error fetching logs",
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
		// --follow tests
		{
			name:                 "follow validates workspace and attempts agent dial",
			args:                 []string{taskID.String(), "--follow"},
			expectError:          "dial workspace agent",
			expectStderrContains: "Connecting to task workspace agent",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, makeTaskWorkspace())
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			name:                 "follow works with task name",
			args:                 []string{taskName, "--follow"},
			expectError:          "dial workspace agent",
			expectStderrContains: "Connecting to task workspace agent",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/users/me/workspace/%s", taskName):
						httpapi.Write(ctx, w, http.StatusOK, makeTaskWorkspace())
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						httpapi.Write(ctx, w, http.StatusOK, makeTaskWorkspace())
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			name:        "follow fails for non-AI task workspace",
			args:        []string{taskID.String(), "--follow"},
			expectError: "not configured as an AI task",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						ws := makeTaskWorkspace()
						notAITask := false
						ws.LatestBuild.HasAITask = &notAITask
						httpapi.Write(ctx, w, http.StatusOK, ws)
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			name:        "follow fails for missing sidebar app",
			args:        []string{taskID.String(), "--follow"},
			expectError: "not configured with a sidebar app",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						ws := makeTaskWorkspace()
						ws.LatestBuild.AITaskSidebarAppID = nil
						httpapi.Write(ctx, w, http.StatusOK, ws)
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			name:        "follow fails for unhealthy app",
			args:        []string{taskID.String(), "--follow"},
			expectError: "unhealthy",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						ws := makeTaskWorkspace()
						ws.LatestBuild.Resources[0].Agents[0].Apps[0].Health = codersdk.WorkspaceAppHealthUnhealthy
						httpapi.Write(ctx, w, http.StatusOK, ws)
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			name:        "follow fails for initializing app",
			args:        []string{taskID.String(), "--follow"},
			expectError: "initializing",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/workspaces/%s", taskID.String()):
						ws := makeTaskWorkspace()
						ws.LatestBuild.Resources[0].Agents[0].Apps[0].Health = codersdk.WorkspaceAppHealthInitializing
						httpapi.Write(ctx, w, http.StatusOK, ws)
					default:
						t.Logf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				srv    = httptest.NewServer(tt.handler(t, ctx))
				client = codersdk.New(testutil.MustURL(t, srv.URL))
				args   = []string{"exp", "task", "logs"}
				stdout strings.Builder
				stderr strings.Builder
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Stdout = &stdout
			inv.Stderr = &stderr
			clitest.SetupConfig(t, client, root)

			err = inv.WithContext(ctx).Run()
			if tt.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectError)
			}

			if tt.expectStderrContains != "" {
				assert.Contains(t, stderr.String(), tt.expectStderrContains)
			}

			if tt.expectTable != "" {
				if diff := tableDiff(tt.expectTable, stdout.String()); diff != "" {
					t.Errorf("unexpected output diff (-want +got):\n%s", diff)
				}
			}

			if tt.expectLogs != nil {
				var logs []codersdk.TaskLogEntry
				err = json.NewDecoder(strings.NewReader(stdout.String())).Decode(&logs)
				require.NoError(t, err)

				assert.Equal(t, tt.expectLogs, logs)
			}
		})
	}
}
