package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args         []string
		expectOutput string
		expectError  string
		hf           func(context.Context, time.Time) func(http.ResponseWriter, *http.Request)
	}{
		{
			args:        []string{"doesnotexist"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			hf: func(ctx context.Context, _ time.Time) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/experimental/tasks/me/doesnotexist":
						httpapi.ResourceNotFound(w)
						return
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args: []string{"exists"},
			expectOutput: `STATE CHANGED  STATUS  HEALTHY  STATE    MESSAGE
0s ago         active  true     working  Thinking furiously...`,
			hf: func(ctx context.Context, now time.Time) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/experimental/tasks/me/exists":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
							ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
							WorkspaceStatus: codersdk.WorkspaceStatusRunning,
							CreatedAt:       now,
							UpdatedAt:       now,
							CurrentState: &codersdk.TaskStateEntry{
								State:     codersdk.TaskStateWorking,
								Timestamp: now,
								Message:   "Thinking furiously...",
							},
							WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
								Healthy: true,
							},
							WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
							Status:                  codersdk.TaskStatusActive,
						})
						return
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args: []string{"exists", "--watch"},
			expectOutput: `STATE CHANGED  STATUS   HEALTHY  STATE  MESSAGE
5s ago         pending  true
4s ago         initializing  true
4s ago         active  true
3s ago         active  true     working  Reticulating splines...
2s ago         active  true     complete  Splines reticulated successfully!`,
			hf: func(ctx context.Context, now time.Time) func(http.ResponseWriter, *http.Request) {
				var calls atomic.Int64
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/experimental/tasks/me/exists":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
							ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
							Name:            "exists",
							OwnerName:       "me",
							WorkspaceStatus: codersdk.WorkspaceStatusPending,
							CreatedAt:       now.Add(-5 * time.Second),
							UpdatedAt:       now.Add(-5 * time.Second),
							WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
								Healthy: true,
							},
							WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
							Status:                  codersdk.TaskStatusPending,
						})
						return
					case "/api/experimental/tasks/me/11111111-1111-1111-1111-111111111111":
						defer calls.Add(1)
						switch calls.Load() {
						case 0:
							httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
								ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
								Name:            "exists",
								OwnerName:       "me",
								WorkspaceStatus: codersdk.WorkspaceStatusRunning,
								CreatedAt:       now.Add(-5 * time.Second),
								UpdatedAt:       now.Add(-4 * time.Second),
								WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
									Healthy: true,
								},
								WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
								Status:                  codersdk.TaskStatusInitializing,
							})
							return
						case 1:
							httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
								ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
								WorkspaceStatus: codersdk.WorkspaceStatusRunning,
								CreatedAt:       now.Add(-5 * time.Second),
								WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
									Healthy: true,
								},
								WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
								UpdatedAt:               now.Add(-4 * time.Second),
								Status:                  codersdk.TaskStatusActive,
							})
							return
						case 2:
							httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
								ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
								WorkspaceStatus: codersdk.WorkspaceStatusRunning,
								CreatedAt:       now.Add(-5 * time.Second),
								UpdatedAt:       now.Add(-4 * time.Second),
								WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
									Healthy: true,
								},
								WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
								CurrentState: &codersdk.TaskStateEntry{
									State:     codersdk.TaskStateWorking,
									Timestamp: now.Add(-3 * time.Second),
									Message:   "Reticulating splines...",
								},
								Status: codersdk.TaskStatusActive,
							})
							return
						case 3:
							httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
								ID:              uuid.MustParse("11111111-1111-1111-1111-111111111111"),
								WorkspaceStatus: codersdk.WorkspaceStatusRunning,
								CreatedAt:       now.Add(-5 * time.Second),
								UpdatedAt:       now.Add(-4 * time.Second),
								WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
									Healthy: true,
								},
								WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
								CurrentState: &codersdk.TaskStateEntry{
									State:     codersdk.TaskStateComplete,
									Timestamp: now.Add(-2 * time.Second),
									Message:   "Splines reticulated successfully!",
								},
								Status: codersdk.TaskStatusActive,
							})
							return
						default:
							httpapi.InternalServerError(w, xerrors.New("too many calls!"))
							return
						}
					default:
						httpapi.InternalServerError(w, xerrors.Errorf("unexpected path: %q", r.URL.Path))
						return
					}
				}
			},
		},
		{
			args: []string{"exists", "--output", "json"},
			expectOutput: `{
  "id": "11111111-1111-1111-1111-111111111111",
  "organization_id": "00000000-0000-0000-0000-000000000000",
  "owner_id": "00000000-0000-0000-0000-000000000000",
  "owner_name": "me",
  "name": "exists",
  "display_name": "Task exists",
  "template_id": "00000000-0000-0000-0000-000000000000",
  "template_version_id": "00000000-0000-0000-0000-000000000000",
  "template_name": "",
  "template_display_name": "",
  "template_icon": "",
  "workspace_id": null,
  "workspace_name": "",
  "workspace_status": "running",
  "workspace_agent_id": null,
  "workspace_agent_lifecycle": "ready",
  "workspace_agent_health": {
    "healthy": true
  },
  "workspace_app_id": null,
  "initial_prompt": "",
  "status": "active",
  "current_state": {
    "timestamp": "2025-08-26T12:34:57Z",
    "state": "working",
    "message": "Thinking furiously...",
    "uri": ""
  },
  "created_at": "2025-08-26T12:34:56Z",
  "updated_at": "2025-08-26T12:34:56Z"
}`,
			hf: func(ctx context.Context, now time.Time) func(http.ResponseWriter, *http.Request) {
				ts := time.Date(2025, 8, 26, 12, 34, 56, 0, time.UTC)
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/experimental/tasks/me/exists":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
							ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
							Name:        "exists",
							DisplayName: "Task exists",
							OwnerName:   "me",
							WorkspaceAgentHealth: &codersdk.WorkspaceAgentHealth{
								Healthy: true,
							},
							WorkspaceAgentLifecycle: ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
							WorkspaceStatus:         codersdk.WorkspaceStatusRunning,
							CreatedAt:               ts,
							UpdatedAt:               ts,
							CurrentState: &codersdk.TaskStateEntry{
								State:     codersdk.TaskStateWorking,
								Timestamp: ts.Add(time.Second),
								Message:   "Thinking furiously...",
							},
							Status: codersdk.TaskStatusActive,
						})
						return
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
	} {
		t.Run(strings.Join(tc.args, ","), func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				now    = time.Now().UTC() // TODO: replace with quartz
				srv    = httptest.NewServer(http.HandlerFunc(tc.hf(ctx, now)))
				client = codersdk.New(testutil.MustURL(t, srv.URL))
				sb     = strings.Builder{}
				args   = []string{"exp", "task", "status", "--watch-interval", testutil.IntervalFast.String()}
			)

			t.Cleanup(srv.Close)
			args = append(args, tc.args...)
			inv, root := clitest.New(t, args...)
			inv.Stdout = &sb
			inv.Stderr = &sb
			clitest.SetupConfig(t, client, root)
			err := inv.WithContext(ctx).Run()
			if tc.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectError)
			}
			if diff := tableDiff(tc.expectOutput, sb.String()); diff != "" {
				t.Errorf("unexpected output diff (-want +got):\n%s", diff)
			}
		})
	}
}

func tableDiff(want, got string) string {
	var gotTrimmed strings.Builder
	for _, line := range strings.Split(got, "\n") {
		_, _ = gotTrimmed.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return cmp.Diff(strings.TrimSpace(want), strings.TrimSpace(gotTrimmed.String()))
}
