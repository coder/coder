package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_TaskStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args         []string
		expectOutput string
		expectError  string
		hf           func(context.Context) func(http.ResponseWriter, *http.Request)
	}{
		{
			args:        []string{"doesnotexist"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			hf: func(ctx context.Context) func(w http.ResponseWriter, r *http.Request) {
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
			args:        []string{"err-fetching-workspace"},
			expectError: assert.AnError.Error(),
			hf: func(ctx context.Context) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/err-fetching-workspace":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						})
					case "/api/experimental/tasks/me/11111111-1111-1111-1111-111111111111":
						httpapi.InternalServerError(w, assert.AnError)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:         []string{"exists"},
			expectOutput: "frobnicating, unknown\n",
			hf: func(ctx context.Context) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/exists":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						})
					case "/api/experimental/tasks/me/11111111-1111-1111-1111-111111111111":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
							ID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
							Status: codersdk.WorkspaceStatus("frobnicating"),
						})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:         []string{"exists", "--follow"},
			expectOutput: "running, working\nstopped, completed\n",
			hf: func(ctx context.Context) func(http.ResponseWriter, *http.Request) {
				var c atomic.Int64
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/workspace/exists":
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Workspace{
							ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						})
					case "/api/experimental/tasks/me/11111111-1111-1111-1111-111111111111":
						if c.Load() < 2 {
							httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
								ID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
								Status: codersdk.WorkspaceStatusRunning,
								CurrentState: &codersdk.TaskStateEntry{
									State: codersdk.TaskStateWorking,
								},
							})
							c.Add(1)
							return
						}
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Task{
							ID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
							Status: codersdk.WorkspaceStatusStopped,
							CurrentState: &codersdk.TaskStateEntry{
								State: codersdk.TaskStateCompleted,
							},
						})
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
				srv    = httptest.NewServer(http.HandlerFunc(tc.hf(ctx)))
				client = new(codersdk.Client)
				sb     = strings.Builder{}
				args   = []string{"exp", "task", "status"}
			)

			t.Cleanup(srv.Close)
			client.URL = testutil.MustURL(t, srv.URL)
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
			assert.Equal(t, tc.expectOutput, sb.String())
		})
	}
}
