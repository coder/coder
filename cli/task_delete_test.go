package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestExpTaskDelete(t *testing.T) {
	t.Parallel()

	type testCounters struct {
		deleteCalls  atomic.Int64
		nameResolves atomic.Int64
	}
	type handlerBuilder func(c *testCounters) http.HandlerFunc

	type testCase struct {
		name               string
		args               []string
		promptYes          bool
		wantErr            bool
		wantDeleteCalls    int64
		wantNameResolves   int64
		wantDeletedMessage int
		buildHandler       handlerBuilder
	}

	const (
		id1 = "11111111-1111-1111-1111-111111111111"
		id2 = "22222222-2222-2222-2222-222222222222"
		id3 = "33333333-3333-3333-3333-333333333333"
		id4 = "44444444-4444-4444-4444-444444444444"
		id5 = "55555555-5555-5555-5555-555555555555"
	)

	cases := []testCase{
		{
			name:      "Prompted_ByName_OK",
			args:      []string{"exists"},
			promptYes: true,
			buildHandler: func(c *testCounters) http.HandlerFunc {
				taskID := uuid.MustParse(id1)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/exists":
						c.nameResolves.Add(1)
						httpapi.Write(r.Context(), w, http.StatusOK,
							codersdk.Task{
								ID:        taskID,
								Name:      "exists",
								OwnerName: "me",
							})
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/tasks/me/"+id1:
						c.deleteCalls.Add(1)
						w.WriteHeader(http.StatusAccepted)
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantDeleteCalls:  1,
			wantNameResolves: 1,
		},
		{
			name:      "Prompted_ByUUID_OK",
			args:      []string{id2},
			promptYes: true,
			buildHandler: func(c *testCounters) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/"+id2:
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        uuid.MustParse(id2),
							OwnerName: "me",
							Name:      "uuid-task",
						})
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/tasks/me/"+id2:
						c.deleteCalls.Add(1)
						w.WriteHeader(http.StatusAccepted)
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantDeleteCalls: 1,
		},
		{
			name: "Multiple_YesFlag",
			args: []string{"--yes", "first", id4},
			buildHandler: func(c *testCounters) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/first":
						c.nameResolves.Add(1)
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        uuid.MustParse(id3),
							Name:      "first",
							OwnerName: "me",
						})
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/"+id4:
						c.nameResolves.Add(1)
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        uuid.MustParse(id4),
							OwnerName: "me",
							Name:      "uuid-task-4",
						})
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/tasks/me/"+id3:
						c.deleteCalls.Add(1)
						w.WriteHeader(http.StatusAccepted)
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/tasks/me/"+id4:
						c.deleteCalls.Add(1)
						w.WriteHeader(http.StatusAccepted)
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantDeleteCalls:    2,
			wantNameResolves:   2,
			wantDeletedMessage: 2,
		},
		{
			name:    "ResolveNameError",
			args:    []string{"doesnotexist"},
			wantErr: true,
			buildHandler: func(_ *testCounters) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks" && r.URL.Query().Get("q") == "owner:\"me\"":
						httpapi.Write(r.Context(), w, http.StatusOK, struct {
							Tasks []codersdk.Task `json:"tasks"`
							Count int             `json:"count"`
						}{
							Tasks: []codersdk.Task{},
							Count: 0,
						})
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
		},
		{
			name:      "DeleteError",
			args:      []string{"bad"},
			promptYes: true,
			wantErr:   true,
			buildHandler: func(c *testCounters) http.HandlerFunc {
				taskID := uuid.MustParse(id5)
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/me/bad":
						c.nameResolves.Add(1)
						httpapi.Write(r.Context(), w, http.StatusOK, codersdk.Task{
							ID:        taskID,
							Name:      "bad",
							OwnerName: "me",
						})
					case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/tasks/me/bad":
						httpapi.InternalServerError(w, xerrors.New("boom"))
					default:
						httpapi.InternalServerError(w, xerrors.New("unwanted path: "+r.Method+" "+r.URL.Path))
					}
				}
			},
			wantNameResolves: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			var counters testCounters
			srv := httptest.NewServer(tc.buildHandler(&counters))
			t.Cleanup(srv.Close)

			client := codersdk.New(testutil.MustURL(t, srv.URL))

			args := append([]string{"task", "delete"}, tc.args...)
			inv, root := clitest.New(t, args...)
			inv = inv.WithContext(ctx)
			clitest.SetupConfig(t, client, root)

			var runErr error
			var outBuf bytes.Buffer
			if tc.promptYes {
				pty := ptytest.New(t).Attach(inv)
				w := clitest.StartWithWaiter(t, inv)
				pty.ExpectMatch("Delete these tasks:")
				pty.WriteLine("yes")
				runErr = w.Wait()
				outBuf.Write(pty.ReadAll())
			} else {
				inv.Stdout = &outBuf
				inv.Stderr = &outBuf
				runErr = inv.Run()
			}

			if tc.wantErr {
				require.Error(t, runErr)
			} else {
				require.NoError(t, runErr)
			}

			require.Equal(t, tc.wantDeleteCalls, counters.deleteCalls.Load(), "wrong delete call count")
			require.Equal(t, tc.wantNameResolves, counters.nameResolves.Load(), "wrong name resolve count")

			if tc.wantDeletedMessage > 0 {
				output := outBuf.String()
				require.GreaterOrEqual(t, strings.Count(output, "Deleted task"), tc.wantDeletedMessage)
			}
		})
	}
}
