package agentcontainers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fakeLister implements the agentcontainers.Lister interface for
// testing.
type fakeLister struct {
	containers codersdk.WorkspaceAgentListContainersResponse
	err        error
}

func (f *fakeLister) List(_ context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return f.containers, f.err
}

// fakeDevcontainerCLI implements the agentcontainers.DevcontainerCLI
// interface for testing.
type fakeDevcontainerCLI struct {
	id  string
	err error
}

func (f *fakeDevcontainerCLI) Up(_ context.Context, _, _ string, _ ...agentcontainers.DevcontainerCLIUpOptions) (string, error) {
	return f.id, f.err
}

// fakeWatcher implements the watcher.Watcher interface for testing.
// It allows controlling what events are sent and when.
type fakeWatcher struct {
	t           testing.TB
	events      chan *fsnotify.Event
	closeNotify chan struct{}
	addedPaths  []string
	closed      bool
	nextCalled  chan struct{}
	nextErr     error
	closeErr    error
}

func newFakeWatcher(t testing.TB) *fakeWatcher {
	return &fakeWatcher{
		t:           t,
		events:      make(chan *fsnotify.Event, 10), // Buffered to avoid blocking tests.
		closeNotify: make(chan struct{}),
		addedPaths:  make([]string, 0),
		nextCalled:  make(chan struct{}, 1),
	}
}

func (w *fakeWatcher) Add(file string) error {
	w.addedPaths = append(w.addedPaths, file)
	return nil
}

func (w *fakeWatcher) Remove(file string) error {
	for i, path := range w.addedPaths {
		if path == file {
			w.addedPaths = append(w.addedPaths[:i], w.addedPaths[i+1:]...)
			break
		}
	}
	return nil
}

func (w *fakeWatcher) clearNext() {
	select {
	case <-w.nextCalled:
	default:
	}
}

func (w *fakeWatcher) waitNext(ctx context.Context) bool {
	select {
	case <-w.t.Context().Done():
		return false
	case <-ctx.Done():
		return false
	case <-w.closeNotify:
		return false
	case <-w.nextCalled:
		return true
	}
}

func (w *fakeWatcher) Next(ctx context.Context) (*fsnotify.Event, error) {
	select {
	case w.nextCalled <- struct{}{}:
	default:
	}

	if w.nextErr != nil {
		err := w.nextErr
		w.nextErr = nil
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.closeNotify:
		return nil, xerrors.New("watcher closed")
	case event := <-w.events:
		return event, nil
	}
}

func (w *fakeWatcher) Close() error {
	if w.closed {
		return nil
	}

	w.closed = true
	close(w.closeNotify)
	return w.closeErr
}

// sendEvent sends a file system event through the fake watcher.
func (w *fakeWatcher) sendEventWaitNextCalled(ctx context.Context, event fsnotify.Event) {
	w.clearNext()
	w.events <- &event
	w.waitNext(ctx)
}

func TestAPI(t *testing.T) {
	t.Parallel()

	t.Run("Recreate", func(t *testing.T) {
		t.Parallel()

		validContainer := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/workspace",
				agentcontainers.DevcontainerConfigFileLabel:  "/workspace/.devcontainer/devcontainer.json",
			},
		}

		missingFolderContainer := codersdk.WorkspaceAgentContainer{
			ID:           "missing-folder-container",
			FriendlyName: "missing-folder-container",
			Labels:       map[string]string{},
		}

		tests := []struct {
			name            string
			containerID     string
			lister          *fakeLister
			devcontainerCLI *fakeDevcontainerCLI
			wantStatus      int
			wantBody        string
		}{
			{
				name:            "Missing ID",
				containerID:     "",
				lister:          &fakeLister{},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusBadRequest,
				wantBody:        "Missing container ID or name",
			},
			{
				name:        "List error",
				containerID: "container-id",
				lister: &fakeLister{
					err: xerrors.New("list error"),
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusInternalServerError,
				wantBody:        "Could not list containers",
			},
			{
				name:        "Container not found",
				containerID: "nonexistent-container",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusNotFound,
				wantBody:        "Container not found",
			},
			{
				name:        "Missing workspace folder label",
				containerID: "missing-folder-container",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{missingFolderContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusBadRequest,
				wantBody:        "Missing workspace folder label",
			},
			{
				name:        "Devcontainer CLI error",
				containerID: "container-id",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{
					err: xerrors.New("devcontainer CLI error"),
				},
				wantStatus: http.StatusInternalServerError,
				wantBody:   "Could not recreate devcontainer",
			},
			{
				name:        "OK",
				containerID: "container-id",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusNoContent,
				wantBody:        "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

				// Setup router with the handler under test.
				r := chi.NewRouter()
				api := agentcontainers.NewAPI(
					logger,
					agentcontainers.WithLister(tt.lister),
					agentcontainers.WithDevcontainerCLI(tt.devcontainerCLI),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				)
				defer api.Close()
				r.Mount("/", api.Routes())

				// Simulate HTTP request to the recreate endpoint.
				req := httptest.NewRequest(http.MethodPost, "/"+tt.containerID+"/recreate", nil)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				// Check the response status code and body.
				require.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
				if tt.wantBody != "" {
					assert.Contains(t, rec.Body.String(), tt.wantBody, "response body mismatch")
				} else if tt.wantStatus == http.StatusNoContent {
					assert.Empty(t, rec.Body.String(), "expected empty response body")
				}
			})
		}
	})

	t.Run("List devcontainers", func(t *testing.T) {
		t.Parallel()

		knownDevcontainerID1 := uuid.New()
		knownDevcontainerID2 := uuid.New()

		knownDevcontainers := []codersdk.WorkspaceAgentDevcontainer{
			{
				ID:              knownDevcontainerID1,
				Name:            "known-devcontainer-1",
				WorkspaceFolder: "/workspace/known1",
				ConfigPath:      "/workspace/known1/.devcontainer/devcontainer.json",
			},
			{
				ID:              knownDevcontainerID2,
				Name:            "known-devcontainer-2",
				WorkspaceFolder: "/workspace/known2",
				// No config path intentionally.
			},
		}

		tests := []struct {
			name               string
			lister             *fakeLister
			knownDevcontainers []codersdk.WorkspaceAgentDevcontainer
			wantStatus         int
			wantCount          int
			verify             func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer)
		}{
			{
				name: "List error",
				lister: &fakeLister{
					err: xerrors.New("list error"),
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name:       "Empty containers",
				lister:     &fakeLister{},
				wantStatus: http.StatusOK,
				wantCount:  0,
			},
			{
				name: "Only known devcontainers, no containers",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{},
					},
				},
				knownDevcontainers: knownDevcontainers,
				wantStatus:         http.StatusOK,
				wantCount:          2,
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					for _, dc := range devcontainers {
						assert.False(t, dc.Running, "devcontainer should not be running")
						assert.Nil(t, dc.Container, "devcontainer should not have container reference")
					}
				},
			},
			{
				name: "Runtime-detected devcontainer",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{
							{
								ID:           "runtime-container-1",
								FriendlyName: "runtime-container-1",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/runtime1",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/runtime1/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "not-a-devcontainer",
								FriendlyName: "not-a-devcontainer",
								Running:      true,
								Labels:       map[string]string{},
							},
						},
					},
				},
				wantStatus: http.StatusOK,
				wantCount:  1,
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					dc := devcontainers[0]
					assert.Equal(t, "/workspace/runtime1", dc.WorkspaceFolder)
					assert.True(t, dc.Running)
					require.NotNil(t, dc.Container)
					assert.Equal(t, "runtime-container-1", dc.Container.ID)
				},
			},
			{
				name: "Mixed known and runtime-detected devcontainers",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{
							{
								ID:           "known-container-1",
								FriendlyName: "known-container-1",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/known1",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/known1/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "runtime-container-1",
								FriendlyName: "runtime-container-1",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/runtime1",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/runtime1/.devcontainer/devcontainer.json",
								},
							},
						},
					},
				},
				knownDevcontainers: knownDevcontainers,
				wantStatus:         http.StatusOK,
				wantCount:          3, // 2 known + 1 runtime
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					known1 := mustFindDevcontainerByPath(t, devcontainers, "/workspace/known1")
					known2 := mustFindDevcontainerByPath(t, devcontainers, "/workspace/known2")
					runtime1 := mustFindDevcontainerByPath(t, devcontainers, "/workspace/runtime1")

					assert.True(t, known1.Running)
					assert.False(t, known2.Running)
					assert.True(t, runtime1.Running)

					require.NotNil(t, known1.Container)
					assert.Nil(t, known2.Container)
					require.NotNil(t, runtime1.Container)

					assert.Equal(t, "known-container-1", known1.Container.ID)
					assert.Equal(t, "runtime-container-1", runtime1.Container.ID)
				},
			},
			{
				name: "Both running and non-running containers have container references",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{
							{
								ID:           "running-container",
								FriendlyName: "running-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/running",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/running/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "non-running-container",
								FriendlyName: "non-running-container",
								Running:      false,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/non-running",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/non-running/.devcontainer/devcontainer.json",
								},
							},
						},
					},
				},
				wantStatus: http.StatusOK,
				wantCount:  2,
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					running := mustFindDevcontainerByPath(t, devcontainers, "/workspace/running")
					nonRunning := mustFindDevcontainerByPath(t, devcontainers, "/workspace/non-running")

					assert.True(t, running.Running)
					assert.False(t, nonRunning.Running)

					require.NotNil(t, running.Container, "running container should have container reference")
					require.NotNil(t, nonRunning.Container, "non-running container should have container reference")

					assert.Equal(t, "running-container", running.Container.ID)
					assert.Equal(t, "non-running-container", nonRunning.Container.ID)
				},
			},
			{
				name: "Config path update",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{
							{
								ID:           "known-container-2",
								FriendlyName: "known-container-2",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/known2",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/known2/.devcontainer/devcontainer.json",
								},
							},
						},
					},
				},
				knownDevcontainers: knownDevcontainers,
				wantStatus:         http.StatusOK,
				wantCount:          2,
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					var dc2 *codersdk.WorkspaceAgentDevcontainer
					for i := range devcontainers {
						if devcontainers[i].ID == knownDevcontainerID2 {
							dc2 = &devcontainers[i]
							break
						}
					}
					require.NotNil(t, dc2, "missing devcontainer with ID %s", knownDevcontainerID2)
					assert.True(t, dc2.Running)
					assert.NotEmpty(t, dc2.ConfigPath)
					require.NotNil(t, dc2.Container)
					assert.Equal(t, "known-container-2", dc2.Container.ID)
				},
			},
			{
				name: "Name generation and uniqueness",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{
							{
								ID:           "project1-container",
								FriendlyName: "project1-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/project",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/project/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "project2-container",
								FriendlyName: "project2-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/home/user/project",
									agentcontainers.DevcontainerConfigFileLabel:  "/home/user/project/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "project3-container",
								FriendlyName: "project3-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/var/lib/project",
									agentcontainers.DevcontainerConfigFileLabel:  "/var/lib/project/.devcontainer/devcontainer.json",
								},
							},
						},
					},
				},
				knownDevcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              uuid.New(),
						Name:            "project", // This will cause uniqueness conflicts.
						WorkspaceFolder: "/usr/local/project",
						ConfigPath:      "/usr/local/project/.devcontainer/devcontainer.json",
					},
				},
				wantStatus: http.StatusOK,
				wantCount:  4, // 1 known + 3 runtime
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					names := make(map[string]int)
					for _, dc := range devcontainers {
						names[dc.Name]++
						assert.NotEmpty(t, dc.Name, "devcontainer name should not be empty")
					}

					for name, count := range names {
						assert.Equal(t, 1, count, "name '%s' appears %d times, should be unique", name, count)
					}
					assert.Len(t, names, 4, "should have four unique devcontainer names")
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

				// Setup router with the handler under test.
				r := chi.NewRouter()
				apiOptions := []agentcontainers.Option{
					agentcontainers.WithLister(tt.lister),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				}

				if len(tt.knownDevcontainers) > 0 {
					apiOptions = append(apiOptions, agentcontainers.WithDevcontainers(tt.knownDevcontainers))
				}

				api := agentcontainers.NewAPI(logger, apiOptions...)
				defer api.Close()
				r.Mount("/", api.Routes())

				req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				// Check the response status code.
				require.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
				if tt.wantStatus != http.StatusOK {
					return
				}

				var response codersdk.WorkspaceAgentDevcontainersResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				require.NoError(t, err, "unmarshal response failed")

				// Verify the number of devcontainers in the response.
				assert.Len(t, response.Devcontainers, tt.wantCount, "wrong number of devcontainers")

				// Run custom verification if provided.
				if tt.verify != nil && len(response.Devcontainers) > 0 {
					tt.verify(t, response.Devcontainers)
				}
			})
		}
	})

	t.Run("FileWatcher", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)

		startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		mClock := quartz.NewMock(t)
		mClock.Set(startTime)
		fWatcher := newFakeWatcher(t)

		// Create a fake container with a config file.
		configPath := "/workspace/project/.devcontainer/devcontainer.json"
		container := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Running:      true,
			CreatedAt:    startTime.Add(-1 * time.Hour), // Created 1 hour before test start.
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/workspace/project",
				agentcontainers.DevcontainerConfigFileLabel:  configPath,
			},
		}

		fLister := &fakeLister{
			containers: codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{container},
			},
		}

		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		api := agentcontainers.NewAPI(
			logger,
			agentcontainers.WithLister(fLister),
			agentcontainers.WithWatcher(fWatcher),
			agentcontainers.WithClock(mClock),
		)
		defer api.Close()

		api.SignalReady()

		r := chi.NewRouter()
		r.Mount("/", api.Routes())

		// Call the list endpoint first to ensure config files are
		// detected and watched.
		req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var response codersdk.WorkspaceAgentDevcontainersResponse
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.False(t, response.Devcontainers[0].Dirty,
			"container should not be marked as dirty initially")

		// Verify the watcher is watching the config file.
		assert.Contains(t, fWatcher.addedPaths, configPath,
			"watcher should be watching the container's config file")

		// Make sure the start loop has been called.
		fWatcher.waitNext(ctx)

		// Send a file modification event and check if the container is
		// marked dirty.
		fWatcher.sendEventWaitNextCalled(ctx, fsnotify.Event{
			Name: configPath,
			Op:   fsnotify.Write,
		})

		mClock.Advance(time.Minute).MustWait(ctx)

		// Check if the container is marked as dirty.
		req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.True(t, response.Devcontainers[0].Dirty,
			"container should be marked as dirty after config file was modified")

		mClock.Advance(time.Minute).MustWait(ctx)

		container.ID = "new-container-id" // Simulate a new container ID after recreation.
		container.FriendlyName = "new-container-name"
		container.CreatedAt = mClock.Now() // Update the creation time.
		fLister.containers.Containers = []codersdk.WorkspaceAgentContainer{container}

		// Check if dirty flag is cleared.
		req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.False(t, response.Devcontainers[0].Dirty,
			"dirty flag should be cleared after container recreation")
	})
}

// mustFindDevcontainerByPath returns the devcontainer with the given workspace
// folder path. It fails the test if no matching devcontainer is found.
func mustFindDevcontainerByPath(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer, path string) codersdk.WorkspaceAgentDevcontainer {
	t.Helper()

	for i := range devcontainers {
		if devcontainers[i].WorkspaceFolder == path {
			return devcontainers[i]
		}
	}

	require.Failf(t, "no devcontainer found with workspace folder %q", path)
	return codersdk.WorkspaceAgentDevcontainer{} // Unreachable, but required for compilation
}
