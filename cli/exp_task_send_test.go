package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskSend(t *testing.T) {
	t.Parallel()

	var (
		taskName = "task-workspace"
		taskID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	)

	tests := []struct {
		args        []string
		stdin       string
		expectError string
		handler     func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			args: []string{taskName, "carry on with the task"},
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/users/me/workspace/%s", taskName):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/send", taskID.String()):
						var req codersdk.TaskSendRequest
						if !httpapi.Read(ctx, w, r, &req) {
							return
						}

						assert.Equal(t, "carry on with the task", req.Input)

						httpapi.Write(ctx, w, http.StatusNoContent, nil)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args: []string{taskID.String(), "carry on with the task"},
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/send", taskID.String()):
						var req codersdk.TaskSendRequest
						if !httpapi.Read(ctx, w, r, &req) {
							return
						}

						assert.Equal(t, "carry on with the task", req.Input)

						httpapi.Write(ctx, w, http.StatusNoContent, nil)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:  []string{taskName, "--stdin"},
			stdin: "carry on with the task",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/v2/users/me/workspace/%s", taskName):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: taskID,
						})
					case fmt.Sprintf("/api/experimental/tasks/me/%s/send", taskID.String()):
						var req codersdk.TaskSendRequest
						if !httpapi.Read(ctx, w, r, &req) {
							return
						}

						assert.Equal(t, "carry on with the task", req.Input)

						httpapi.Write(ctx, w, http.StatusNoContent, nil)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"doesnotexist", "some task input"},
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
			args:        []string{uuid.Nil.String(), "some task input"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/send", uuid.Nil.String()):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{uuid.Nil.String(), "some task input"},
			expectError: assert.AnError.Error(),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case fmt.Sprintf("/api/experimental/tasks/me/%s/send", uuid.Nil.String()):
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
				args   = []string{"exp", "task", "send"}
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Stdin = strings.NewReader(tt.stdin)
			clitest.SetupConfig(t, client, root)

			err = inv.WithContext(ctx).Run()
			if tt.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectError)
			}
		})
	}
}
