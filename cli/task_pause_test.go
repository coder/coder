package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpTaskPause(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name          string
		args          []string
		promptYes     bool
		promptNo      bool
		wantErr       string
		wantPausedMsg bool
		buildHandler  func() http.HandlerFunc
	}

	const (
		id1 = "11111111-1111-1111-1111-111111111111"
		id2 = "22222222-2222-2222-2222-222222222222"
		id3 = "33333333-3333-3333-3333-333333333333"
	)

	cases := []testCase{
		{
			name:      "OK_ByName",
			args:      []string{"my-task"},
			promptYes: true,
			buildHandler: func() http.HandlerFunc {
				taskID := uuid.MustParse(id1)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/my-task":
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        taskID,
							Name:      "my-task",
							OwnerName: "me",
						})
					case r.Method == http.MethodPost && r.URL.Path == "/api/experimental/tasks/me/"+id1+"/pause":
						httpapi.Write(r.Context(), w, http.StatusAccepted, codersdk.PauseTaskResponse{})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantPausedMsg: true,
		},
		{
			name:      "OK_ByUUID",
			args:      []string{id2},
			promptYes: true,
			buildHandler: func() http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/"+id2:
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        uuid.MustParse(id2),
							OwnerName: "me",
							Name:      "uuid-task",
						})
					case r.Method == http.MethodPost && r.URL.Path == "/api/experimental/tasks/me/"+id2+"/pause":
						httpapi.Write(r.Context(), w, http.StatusAccepted, codersdk.PauseTaskResponse{})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantPausedMsg: true,
		},
		{
			name: "YesFlag",
			args: []string{"--yes", "my-task"},
			buildHandler: func() http.HandlerFunc {
				taskID := uuid.MustParse(id3)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/my-task":
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        taskID,
							Name:      "my-task",
							OwnerName: "me",
						})
					case r.Method == http.MethodPost && r.URL.Path == "/api/experimental/tasks/me/"+id3+"/pause":
						httpapi.Write(r.Context(), w, http.StatusAccepted, codersdk.PauseTaskResponse{})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantPausedMsg: true,
		},
		{
			name:    "ResolveError",
			args:    []string{"doesnotexist"},
			wantErr: "resolve task",
			buildHandler: func() http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/doesnotexist":
						httpapi.Write(r.Context(), w, http.StatusNotFound, codersdk.Response{
							Message: "Task not found.",
						})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
		},
		{
			name:      "PauseError",
			args:      []string{"bad-task"},
			promptYes: true,
			wantErr:   "pause task",
			buildHandler: func() http.HandlerFunc {
				taskID := uuid.MustParse(id1)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/bad-task":
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        taskID,
							Name:      "bad-task",
							OwnerName: "me",
						})
					case r.Method == http.MethodPost && r.URL.Path == "/api/experimental/tasks/me/"+id1+"/pause":
						httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
							Message: "Internal error.",
							Detail:  "boom",
						})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
		},
		{
			name:     "PromptDeclined",
			args:     []string{"my-task"},
			promptNo: true,
			wantErr:  "canceled",
			buildHandler: func() http.HandlerFunc {
				taskID := uuid.MustParse(id1)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/my-task":
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        taskID,
							Name:      "my-task",
							OwnerName: "me",
						})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			srv := httptest.NewServer(tc.buildHandler())
			t.Cleanup(srv.Close)

			client := codersdk.New(testutil.MustURL(t, srv.URL))

			args := append([]string{"task", "pause"}, tc.args...)
			inv, root := clitest.New(t, args...)
			inv = inv.WithContext(ctx)
			clitest.SetupConfig(t, client, root)

			var runErr error
			var outBuf bytes.Buffer
			if tc.promptYes || tc.promptNo {
				pty := ptytest.New(t).Attach(inv)
				w := clitest.StartWithWaiter(t, inv)
				pty.ExpectMatchContext(ctx, "Pause task")
				if tc.promptYes {
					pty.WriteLine("yes")
				} else {
					pty.WriteLine("no")
				}
				if tc.wantPausedMsg {
					pty.ExpectMatchContext(ctx, "has been paused")
				}
				runErr = w.Wait()
			} else {
				inv.Stdout = &outBuf
				inv.Stderr = &outBuf
				runErr = inv.Run()
			}

			if tc.wantErr != "" {
				require.ErrorContains(t, runErr, tc.wantErr)
			} else {
				require.NoError(t, runErr)
			}

			// For non-PTY tests, verify output from the buffer.
			if tc.wantPausedMsg && !tc.promptYes && !tc.promptNo {
				output := outBuf.String()
				require.Contains(t, output, "has been paused")
			}
		})
	}
}
