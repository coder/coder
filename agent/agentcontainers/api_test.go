package agentcontainers_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentcontainers/acmock"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fakeContainerCLI implements the agentcontainers.ContainerCLI interface for
// testing.
type fakeContainerCLI struct {
	containers codersdk.WorkspaceAgentListContainersResponse
	listErr    error
	arch       string
	archErr    error
	copyErr    error
	execErr    error
}

func (f *fakeContainerCLI) List(_ context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return f.containers, f.listErr
}

func (f *fakeContainerCLI) DetectArchitecture(_ context.Context, _ string) (string, error) {
	return f.arch, f.archErr
}

func (f *fakeContainerCLI) Copy(ctx context.Context, name, src, dst string) error {
	return f.copyErr
}

func (f *fakeContainerCLI) ExecAs(ctx context.Context, name, user string, args ...string) ([]byte, error) {
	return nil, f.execErr
}

// fakeDevcontainerCLI implements the agentcontainers.DevcontainerCLI
// interface for testing.
type fakeDevcontainerCLI struct {
	upID           string
	upErr          error
	upErrC         chan error // If set, send to return err, close to return upErr.
	execErr        error
	execErrC       chan func(cmd string, args ...string) error // If set, send fn to return err, nil or close to return execErr.
	readConfig     agentcontainers.DevcontainerConfig
	readConfigErr  error
	readConfigErrC chan error
}

func (f *fakeDevcontainerCLI) Up(ctx context.Context, _, _ string, _ ...agentcontainers.DevcontainerCLIUpOptions) (string, error) {
	if f.upErrC != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err, ok := <-f.upErrC:
			if ok {
				return f.upID, err
			}
		}
	}
	return f.upID, f.upErr
}

func (f *fakeDevcontainerCLI) Exec(ctx context.Context, _, _ string, cmd string, args []string, _ ...agentcontainers.DevcontainerCLIExecOptions) error {
	if f.execErrC != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case fn, ok := <-f.execErrC:
			if ok && fn != nil {
				return fn(cmd, args...)
			}
		}
	}
	return f.execErr
}

func (f *fakeDevcontainerCLI) ReadConfig(ctx context.Context, _, _ string, _ ...agentcontainers.DevcontainerCLIReadConfigOptions) (agentcontainers.DevcontainerConfig, error) {
	if f.readConfigErrC != nil {
		select {
		case <-ctx.Done():
			return agentcontainers.DevcontainerConfig{}, ctx.Err()
		case err, ok := <-f.readConfigErrC:
			if ok {
				return f.readConfig, err
			}
		}
	}
	return f.readConfig, f.readConfigErr
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

// fakeSubAgentClient implements SubAgentClient for testing purposes.
type fakeSubAgentClient struct {
	agents map[uuid.UUID]agentcontainers.SubAgent
	nextID int

	listErrC   chan error // If set, send to return error, close to return nil.
	created    []agentcontainers.SubAgent
	createErrC chan error // If set, send to return error, close to return nil.
	deleted    []uuid.UUID
	deleteErrC chan error // If set, send to return error, close to return nil.
}

func (m *fakeSubAgentClient) List(ctx context.Context) ([]agentcontainers.SubAgent, error) {
	var listErr error
	if m.listErrC != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err, ok := <-m.listErrC:
			if ok {
				listErr = err
			}
		}
	}
	var agents []agentcontainers.SubAgent
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents, listErr
}

func (m *fakeSubAgentClient) Create(ctx context.Context, agent agentcontainers.SubAgent) (agentcontainers.SubAgent, error) {
	var createErr error
	if m.createErrC != nil {
		select {
		case <-ctx.Done():
			return agentcontainers.SubAgent{}, ctx.Err()
		case err, ok := <-m.createErrC:
			if ok {
				createErr = err
			}
		}
	}
	m.nextID++
	agent.ID = uuid.New()
	agent.AuthToken = uuid.New()
	if m.agents == nil {
		m.agents = make(map[uuid.UUID]agentcontainers.SubAgent)
	}
	m.agents[agent.ID] = agent
	m.created = append(m.created, agent)
	return agent, createErr
}

func (m *fakeSubAgentClient) Delete(ctx context.Context, id uuid.UUID) error {
	var deleteErr error
	if m.deleteErrC != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-m.deleteErrC:
			if ok {
				deleteErr = err
			}
		}
	}
	if m.agents == nil {
		m.agents = make(map[uuid.UUID]agentcontainers.SubAgent)
	}
	delete(m.agents, id)
	m.deleted = append(m.deleted, id)
	return deleteErr
}

func TestAPI(t *testing.T) {
	t.Parallel()

	// List tests the API.getContainers method using a mock
	// implementation. It specifically tests caching behavior.
	t.Run("List", func(t *testing.T) {
		t.Parallel()

		fakeCt := fakeContainer(t)
		fakeCt2 := fakeContainer(t)
		makeResponse := func(cts ...codersdk.WorkspaceAgentContainer) codersdk.WorkspaceAgentListContainersResponse {
			return codersdk.WorkspaceAgentListContainersResponse{Containers: cts}
		}

		type initialDataPayload struct {
			val codersdk.WorkspaceAgentListContainersResponse
			err error
		}

		// Each test case is called multiple times to ensure idempotency
		for _, tc := range []struct {
			name string
			// initialData to be stored in the handler
			initialData initialDataPayload
			// function to set up expectations for the mock
			setupMock func(mcl *acmock.MockContainerCLI, preReq *gomock.Call)
			// expected result
			expected codersdk.WorkspaceAgentListContainersResponse
			// expected error
			expectedErr string
		}{
			{
				name:        "no initial data",
				initialData: initialDataPayload{makeResponse(), nil},
				setupMock: func(mcl *acmock.MockContainerCLI, preReq *gomock.Call) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt), nil).After(preReq).AnyTimes()
				},
				expected: makeResponse(fakeCt),
			},
			{
				name:        "repeat initial data",
				initialData: initialDataPayload{makeResponse(fakeCt), nil},
				expected:    makeResponse(fakeCt),
			},
			{
				name:        "lister error always",
				initialData: initialDataPayload{makeResponse(), assert.AnError},
				expectedErr: assert.AnError.Error(),
			},
			{
				name:        "lister error only during initial data",
				initialData: initialDataPayload{makeResponse(), assert.AnError},
				setupMock: func(mcl *acmock.MockContainerCLI, preReq *gomock.Call) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt), nil).After(preReq).AnyTimes()
				},
				expected: makeResponse(fakeCt),
			},
			{
				name:        "lister error after initial data",
				initialData: initialDataPayload{makeResponse(fakeCt), nil},
				setupMock: func(mcl *acmock.MockContainerCLI, preReq *gomock.Call) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(), assert.AnError).After(preReq).AnyTimes()
				},
				expectedErr: assert.AnError.Error(),
			},
			{
				name:        "updated data",
				initialData: initialDataPayload{makeResponse(fakeCt), nil},
				setupMock: func(mcl *acmock.MockContainerCLI, preReq *gomock.Call) {
					mcl.EXPECT().List(gomock.Any()).Return(makeResponse(fakeCt2), nil).After(preReq).AnyTimes()
				},
				expected: makeResponse(fakeCt2),
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var (
					ctx        = testutil.Context(t, testutil.WaitShort)
					mClock     = quartz.NewMock(t)
					tickerTrap = mClock.Trap().TickerFunc("updaterLoop")
					mCtrl      = gomock.NewController(t)
					mLister    = acmock.NewMockContainerCLI(mCtrl)
					logger     = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
					r          = chi.NewRouter()
				)

				initialDataCall := mLister.EXPECT().List(gomock.Any()).Return(tc.initialData.val, tc.initialData.err)
				if tc.setupMock != nil {
					tc.setupMock(mLister, initialDataCall.Times(1))
				} else {
					initialDataCall.AnyTimes()
				}

				api := agentcontainers.NewAPI(logger,
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(mLister),
					agentcontainers.WithContainerLabelIncludeFilter("this.label.does.not.exist.ignore.devcontainers", "true"),
				)
				defer api.Close()
				r.Mount("/", api.Routes())

				// Make sure the ticker function has been registered
				// before advancing the clock.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				// Initial request returns the initial data.
				req := httptest.NewRequest(http.MethodGet, "/", nil).
					WithContext(ctx)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				if tc.initialData.err != nil {
					got := &codersdk.Error{}
					err := json.NewDecoder(rec.Body).Decode(got)
					require.NoError(t, err, "unmarshal response failed")
					require.ErrorContains(t, got, tc.initialData.err.Error(), "want error")
				} else {
					var got codersdk.WorkspaceAgentListContainersResponse
					err := json.NewDecoder(rec.Body).Decode(&got)
					require.NoError(t, err, "unmarshal response failed")
					require.Equal(t, tc.initialData.val, got, "want initial data")
				}

				// Advance the clock to run updaterLoop.
				_, aw := mClock.AdvanceNext()
				aw.MustWait(ctx)

				// Second request returns the updated data.
				req = httptest.NewRequest(http.MethodGet, "/", nil).
					WithContext(ctx)
				rec = httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				if tc.expectedErr != "" {
					got := &codersdk.Error{}
					err := json.NewDecoder(rec.Body).Decode(got)
					require.NoError(t, err, "unmarshal response failed")
					require.ErrorContains(t, got, tc.expectedErr, "want error")
					return
				}

				var got codersdk.WorkspaceAgentListContainersResponse
				err := json.NewDecoder(rec.Body).Decode(&got)
				require.NoError(t, err, "unmarshal response failed")
				require.Equal(t, tc.expected, got, "want updated data")
			})
		}
	})

	t.Run("Recreate", func(t *testing.T) {
		t.Parallel()

		validContainer := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Running:      true,
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/workspaces",
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
			lister          *fakeContainerCLI
			devcontainerCLI *fakeDevcontainerCLI
			wantStatus      []int
			wantBody        []string
		}{
			{
				name:            "Missing container ID",
				containerID:     "",
				lister:          &fakeContainerCLI{},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusBadRequest},
				wantBody:        []string{"Missing container ID or name"},
			},
			{
				name:        "List error",
				containerID: "container-id",
				lister: &fakeContainerCLI{
					listErr: xerrors.New("list error"),
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusInternalServerError},
				wantBody:        []string{"Could not list containers"},
			},
			{
				name:        "Container not found",
				containerID: "nonexistent-container",
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusNotFound},
				wantBody:        []string{"Container not found"},
			},
			{
				name:        "Missing workspace folder label",
				containerID: "missing-folder-container",
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{missingFolderContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusBadRequest},
				wantBody:        []string{"Missing workspace folder label"},
			},
			{
				name:        "Devcontainer CLI error",
				containerID: "container-id",
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
					arch: "<none>", // Unsupported architecture, don't inject subagent.
				},
				devcontainerCLI: &fakeDevcontainerCLI{
					upErr: xerrors.New("devcontainer CLI error"),
				},
				wantStatus: []int{http.StatusAccepted, http.StatusConflict},
				wantBody:   []string{"Devcontainer recreation initiated", "Devcontainer recreation already in progress"},
			},
			{
				name:        "OK",
				containerID: "container-id",
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
					arch: "<none>", // Unsupported architecture, don't inject subagent.
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusAccepted, http.StatusConflict},
				wantBody:        []string{"Devcontainer recreation initiated", "Devcontainer recreation already in progress"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				require.GreaterOrEqual(t, len(tt.wantStatus), 1, "developer error: at least one status code expected")
				require.Len(t, tt.wantStatus, len(tt.wantBody), "developer error: status and body length mismatch")

				ctx := testutil.Context(t, testutil.WaitShort)

				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				mClock := quartz.NewMock(t)
				mClock.Set(time.Now()).MustWait(ctx)
				tickerTrap := mClock.Trap().TickerFunc("updaterLoop")
				nowRecreateErrorTrap := mClock.Trap().Now("recreate", "errorTimes")
				nowRecreateSuccessTrap := mClock.Trap().Now("recreate", "successTimes")

				tt.devcontainerCLI.upErrC = make(chan error)

				// Setup router with the handler under test.
				r := chi.NewRouter()
				api := agentcontainers.NewAPI(
					logger,
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(tt.lister),
					agentcontainers.WithDevcontainerCLI(tt.devcontainerCLI),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				)
				defer api.Close()
				r.Mount("/", api.Routes())

				// Make sure the ticker function has been registered
				// before advancing the clock.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				for i := range tt.wantStatus {
					// Simulate HTTP request to the recreate endpoint.
					req := httptest.NewRequest(http.MethodPost, "/devcontainers/container/"+tt.containerID+"/recreate", nil).
						WithContext(ctx)
					rec := httptest.NewRecorder()
					r.ServeHTTP(rec, req)

					// Check the response status code and body.
					require.Equal(t, tt.wantStatus[i], rec.Code, "status code mismatch")
					if tt.wantBody[i] != "" {
						assert.Contains(t, rec.Body.String(), tt.wantBody[i], "response body mismatch")
					}
				}

				// Error tests are simple, but the remainder of this test is a
				// bit more involved, closer to an integration test. That is
				// because we must check what state the devcontainer ends up in
				// after the recreation process is initiated and finished.
				if tt.wantStatus[0] != http.StatusAccepted {
					close(tt.devcontainerCLI.upErrC)
					nowRecreateSuccessTrap.Close()
					nowRecreateErrorTrap.Close()
					return
				}

				_, aw := mClock.AdvanceNext()
				aw.MustWait(ctx)

				// Verify the devcontainer is in starting state after recreation
				// request is made.
				req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
					WithContext(ctx)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				require.Equal(t, http.StatusOK, rec.Code, "status code mismatch")
				var resp codersdk.WorkspaceAgentDevcontainersResponse
				t.Log(rec.Body.String())
				err := json.NewDecoder(rec.Body).Decode(&resp)
				require.NoError(t, err, "unmarshal response failed")
				require.Len(t, resp.Devcontainers, 1, "expected one devcontainer in response")
				assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStarting, resp.Devcontainers[0].Status, "devcontainer is not starting")
				require.NotNil(t, resp.Devcontainers[0].Container, "devcontainer should have container reference")
				assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStarting, resp.Devcontainers[0].Container.DevcontainerStatus, "container dc status is not starting")

				// Allow the devcontainer CLI to continue the up process.
				close(tt.devcontainerCLI.upErrC)

				// Ensure the devcontainer ends up in error state if the up call fails.
				if tt.devcontainerCLI.upErr != nil {
					nowRecreateSuccessTrap.Close()
					// The timestamp for the error will be stored, which gives
					// us a good anchor point to know when to do our request.
					nowRecreateErrorTrap.MustWait(ctx).MustRelease(ctx)
					nowRecreateErrorTrap.Close()

					// Advance the clock to run the devcontainer state update routine.
					_, aw = mClock.AdvanceNext()
					aw.MustWait(ctx)

					req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
						WithContext(ctx)
					rec = httptest.NewRecorder()
					r.ServeHTTP(rec, req)

					require.Equal(t, http.StatusOK, rec.Code, "status code mismatch after error")
					err = json.NewDecoder(rec.Body).Decode(&resp)
					require.NoError(t, err, "unmarshal response failed after error")
					require.Len(t, resp.Devcontainers, 1, "expected one devcontainer in response after error")
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusError, resp.Devcontainers[0].Status, "devcontainer is not in an error state after up failure")
					require.NotNil(t, resp.Devcontainers[0].Container, "devcontainer should have container reference after up failure")
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusError, resp.Devcontainers[0].Container.DevcontainerStatus, "container dc status is not error after up failure")
					return
				}

				// Ensure the devcontainer ends up in success state.
				nowRecreateSuccessTrap.MustWait(ctx).MustRelease(ctx)
				nowRecreateSuccessTrap.Close()

				// Advance the clock to run the devcontainer state update routine.
				_, aw = mClock.AdvanceNext()
				aw.MustWait(ctx)

				req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
					WithContext(ctx)
				rec = httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				// Check the response status code and body after recreation.
				require.Equal(t, http.StatusOK, rec.Code, "status code mismatch after recreation")
				t.Log(rec.Body.String())
				err = json.NewDecoder(rec.Body).Decode(&resp)
				require.NoError(t, err, "unmarshal response failed after recreation")
				require.Len(t, resp.Devcontainers, 1, "expected one devcontainer in response after recreation")
				assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, resp.Devcontainers[0].Status, "devcontainer is not running after recreation")
				require.NotNil(t, resp.Devcontainers[0].Container, "devcontainer should have container reference after recreation")
				assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, resp.Devcontainers[0].Container.DevcontainerStatus, "container dc status is not running after recreation")
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
			lister             *fakeContainerCLI
			knownDevcontainers []codersdk.WorkspaceAgentDevcontainer
			wantStatus         int
			wantCount          int
			verify             func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer)
		}{
			{
				name: "List error",
				lister: &fakeContainerCLI{
					listErr: xerrors.New("list error"),
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name:       "Empty containers",
				lister:     &fakeContainerCLI{},
				wantStatus: http.StatusOK,
				wantCount:  0,
			},
			{
				name: "Only known devcontainers, no containers",
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{},
					},
				},
				knownDevcontainers: knownDevcontainers,
				wantStatus:         http.StatusOK,
				wantCount:          2,
				verify: func(t *testing.T, devcontainers []codersdk.WorkspaceAgentDevcontainer) {
					for _, dc := range devcontainers {
						assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStopped, dc.Status, "devcontainer should be stopped")
						assert.Nil(t, dc.Container, "devcontainer should not have container reference")
					}
				},
			},
			{
				name: "Runtime-detected devcontainer",
				lister: &fakeContainerCLI{
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
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, dc.Status)
					require.NotNil(t, dc.Container)
					assert.Equal(t, "runtime-container-1", dc.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, dc.Container.DevcontainerStatus)
				},
			},
			{
				name: "Mixed known and runtime-detected devcontainers",
				lister: &fakeContainerCLI{
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

					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, known1.Status)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStopped, known2.Status)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, runtime1.Status)

					assert.Nil(t, known2.Container)

					require.NotNil(t, known1.Container)
					assert.Equal(t, "known-container-1", known1.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, known1.Container.DevcontainerStatus)
					require.NotNil(t, runtime1.Container)
					assert.Equal(t, "runtime-container-1", runtime1.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, runtime1.Container.DevcontainerStatus)
				},
			},
			{
				name: "Both running and non-running containers have container references",
				lister: &fakeContainerCLI{
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

					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, running.Status)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStopped, nonRunning.Status)

					require.NotNil(t, running.Container, "running container should have container reference")
					assert.Equal(t, "running-container", running.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, running.Container.DevcontainerStatus)

					require.NotNil(t, nonRunning.Container, "non-running container should have container reference")
					assert.Equal(t, "non-running-container", nonRunning.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStopped, nonRunning.Container.DevcontainerStatus)
				},
			},
			{
				name: "Config path update",
				lister: &fakeContainerCLI{
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
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, dc2.Status)
					assert.NotEmpty(t, dc2.ConfigPath)
					require.NotNil(t, dc2.Container)
					assert.Equal(t, "known-container-2", dc2.Container.ID)
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, dc2.Container.DevcontainerStatus)
				},
			},
			{
				name: "Name generation and uniqueness",
				lister: &fakeContainerCLI{
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

				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

				mClock := quartz.NewMock(t)
				mClock.Set(time.Now()).MustWait(testutil.Context(t, testutil.WaitShort))
				tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

				// Setup router with the handler under test.
				r := chi.NewRouter()
				apiOptions := []agentcontainers.Option{
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(tt.lister),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				}

				// Generate matching scripts for the known devcontainers
				// (required to extract log source ID).
				var scripts []codersdk.WorkspaceAgentScript
				for i := range tt.knownDevcontainers {
					scripts = append(scripts, codersdk.WorkspaceAgentScript{
						ID:          tt.knownDevcontainers[i].ID,
						LogSourceID: uuid.New(),
					})
				}
				if len(tt.knownDevcontainers) > 0 {
					apiOptions = append(apiOptions, agentcontainers.WithDevcontainers(tt.knownDevcontainers, scripts))
				}

				api := agentcontainers.NewAPI(logger, apiOptions...)
				defer api.Close()

				r.Mount("/", api.Routes())

				ctx := testutil.Context(t, testutil.WaitShort)

				// Make sure the ticker function has been registered
				// before advancing the clock.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				// Advance the clock to run the updater loop.
				_, aw := mClock.AdvanceNext()
				aw.MustWait(ctx)

				req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
					WithContext(ctx)
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

	t.Run("List devcontainers running then not running", func(t *testing.T) {
		t.Parallel()

		container := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Running:      true,
			CreatedAt:    time.Now().Add(-1 * time.Minute),
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/home/coder/project",
				agentcontainers.DevcontainerConfigFileLabel:  "/home/coder/project/.devcontainer/devcontainer.json",
			},
		}
		dc := codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            "test-devcontainer",
			WorkspaceFolder: "/home/coder/project",
			ConfigPath:      "/home/coder/project/.devcontainer/devcontainer.json",
			Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning, // Corrected enum
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		fLister := &fakeContainerCLI{
			containers: codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{container},
			},
		}
		fWatcher := newFakeWatcher(t)
		mClock := quartz.NewMock(t)
		mClock.Set(time.Now()).MustWait(ctx)
		tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithContainerCLI(fLister),
			agentcontainers.WithWatcher(fWatcher),
			agentcontainers.WithDevcontainers(
				[]codersdk.WorkspaceAgentDevcontainer{dc},
				[]codersdk.WorkspaceAgentScript{{LogSourceID: uuid.New(), ID: dc.ID}},
			),
		)
		defer api.Close()

		// Make sure the ticker function has been registered
		// before advancing any use of mClock.Advance.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Make sure the start loop has been called.
		fWatcher.waitNext(ctx)

		// Simulate a file modification event to make the devcontainer dirty.
		fWatcher.sendEventWaitNextCalled(ctx, fsnotify.Event{
			Name: "/home/coder/project/.devcontainer/devcontainer.json",
			Op:   fsnotify.Write,
		})

		// Initially the devcontainer should be running and dirty.
		req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
			WithContext(ctx)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp1 codersdk.WorkspaceAgentDevcontainersResponse
		err := json.NewDecoder(rec.Body).Decode(&resp1)
		require.NoError(t, err)
		require.Len(t, resp1.Devcontainers, 1)
		require.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, resp1.Devcontainers[0].Status, "devcontainer should be running initially")
		require.True(t, resp1.Devcontainers[0].Dirty, "devcontainer should be dirty initially")
		require.NotNil(t, resp1.Devcontainers[0].Container, "devcontainer should have a container initially")

		// Next, simulate a situation where the container is no longer
		// running.
		fLister.containers.Containers = []codersdk.WorkspaceAgentContainer{}

		// Trigger a refresh which will use the second response from mock
		// lister (no containers).
		_, aw := mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Afterwards the devcontainer should not be running and not dirty.
		req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
			WithContext(ctx)
		rec = httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp2 codersdk.WorkspaceAgentDevcontainersResponse
		err = json.NewDecoder(rec.Body).Decode(&resp2)
		require.NoError(t, err)
		require.Len(t, resp2.Devcontainers, 1)
		require.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStopped, resp2.Devcontainers[0].Status, "devcontainer should not be running after empty list")
		require.False(t, resp2.Devcontainers[0].Dirty, "devcontainer should not be dirty after empty list")
		require.Nil(t, resp2.Devcontainers[0].Container, "devcontainer should not have a container after empty list")
	})

	t.Run("FileWatcher", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

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

		mClock := quartz.NewMock(t)
		mClock.Set(startTime)
		tickerTrap := mClock.Trap().TickerFunc("updaterLoop")
		fWatcher := newFakeWatcher(t)
		fLister := &fakeContainerCLI{
			containers: codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{container},
			},
		}
		fDCCLI := &fakeDevcontainerCLI{}

		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		api := agentcontainers.NewAPI(
			logger,
			agentcontainers.WithDevcontainerCLI(fDCCLI),
			agentcontainers.WithContainerCLI(fLister),
			agentcontainers.WithWatcher(fWatcher),
			agentcontainers.WithClock(mClock),
		)
		defer api.Close()

		r := chi.NewRouter()
		r.Mount("/", api.Routes())

		// Make sure the ticker function has been registered
		// before advancing any use of mClock.Advance.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Call the list endpoint first to ensure config files are
		// detected and watched.
		req := httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
			WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var response codersdk.WorkspaceAgentDevcontainersResponse
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.False(t, response.Devcontainers[0].Dirty,
			"devcontainer should not be marked as dirty initially")
		assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, response.Devcontainers[0].Status, "devcontainer should be running initially")
		require.NotNil(t, response.Devcontainers[0].Container, "container should not be nil")
		assert.False(t, response.Devcontainers[0].Container.DevcontainerDirty,
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

		// Advance the clock to run updaterLoop.
		_, aw := mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Check if the container is marked as dirty.
		req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
			WithContext(ctx)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.True(t, response.Devcontainers[0].Dirty,
			"container should be marked as dirty after config file was modified")
		assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, response.Devcontainers[0].Status, "devcontainer should be running after config file was modified")
		require.NotNil(t, response.Devcontainers[0].Container, "container should not be nil")
		assert.True(t, response.Devcontainers[0].Container.DevcontainerDirty,
			"container should be marked as dirty after config file was modified")

		container.ID = "new-container-id" // Simulate a new container ID after recreation.
		container.FriendlyName = "new-container-name"
		container.CreatedAt = mClock.Now() // Update the creation time.
		fLister.containers.Containers = []codersdk.WorkspaceAgentContainer{container}

		// Advance the clock to run updaterLoop.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Check if dirty flag is cleared.
		req = httptest.NewRequest(http.MethodGet, "/devcontainers", nil).
			WithContext(ctx)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.False(t, response.Devcontainers[0].Dirty,
			"dirty flag should be cleared on the devcontainer after container recreation")
		assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, response.Devcontainers[0].Status, "devcontainer should be running after recreation")
		require.NotNil(t, response.Devcontainers[0].Container, "container should not be nil")
		assert.False(t, response.Devcontainers[0].Container.DevcontainerDirty,
			"dirty flag should be cleared on the container after container recreation")
	})

	t.Run("SubAgentLifecycle", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		var (
			ctx                = testutil.Context(t, testutil.WaitMedium)
			errTestTermination = xerrors.New("test termination")
			logger             = slogtest.Make(t, &slogtest.Options{IgnoredErrorIs: []error{errTestTermination}}).Leveled(slog.LevelDebug)
			mClock             = quartz.NewMock(t)
			mCCLI              = acmock.NewMockContainerCLI(gomock.NewController(t))
			fakeSAC            = &fakeSubAgentClient{
				createErrC: make(chan error, 1),
				deleteErrC: make(chan error, 1),
			}
			fakeDCCLI = &fakeDevcontainerCLI{
				execErrC: make(chan func(cmd string, args ...string) error, 1),
			}

			testContainer = codersdk.WorkspaceAgentContainer{
				ID:           "test-container-id",
				FriendlyName: "test-container",
				Image:        "test-image",
				Running:      true,
				CreatedAt:    time.Now(),
				Labels: map[string]string{
					agentcontainers.DevcontainerLocalFolderLabel: "/workspaces",
					agentcontainers.DevcontainerConfigFileLabel:  "/workspace/.devcontainer/devcontainer.json",
				},
			}
		)

		coderBin, err := os.Executable()
		require.NoError(t, err)

		mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{testContainer},
		}, nil).AnyTimes()
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), "test-container-id").Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), "test-container-id", coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
		)

		mClock.Set(time.Now()).MustWait(ctx)
		tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithContainerCLI(mCCLI),
			agentcontainers.WithWatcher(watcher.NewNoop()),
			agentcontainers.WithSubAgentClient(fakeSAC),
			agentcontainers.WithSubAgentURL("test-subagent-url"),
			agentcontainers.WithDevcontainerCLI(fakeDCCLI),
		)
		defer api.Close()

		// Close before api.Close() defer to avoid deadlock after test.
		defer close(fakeSAC.createErrC)
		defer close(fakeSAC.deleteErrC)
		defer close(fakeDCCLI.execErrC)

		// Allow initial agent creation and injection to succeed.
		testutil.RequireSend(ctx, t, fakeSAC.createErrC, nil)
		testutil.RequireSend(ctx, t, fakeDCCLI.execErrC, func(cmd string, args ...string) error {
			assert.Equal(t, "pwd", cmd)
			assert.Empty(t, args)
			return nil
		}) // Exec pwd.

		// Make sure the ticker function has been registered
		// before advancing the clock.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Ensure we only inject the agent once.
		for i := range 3 {
			_, aw := mClock.AdvanceNext()
			aw.MustWait(ctx)

			t.Logf("Iteration %d: agents created: %d", i+1, len(fakeSAC.created))

			// Verify agent was created.
			require.Len(t, fakeSAC.created, 1)
			assert.Equal(t, "test-container", fakeSAC.created[0].Name)
			assert.Equal(t, "/workspaces", fakeSAC.created[0].Directory)
			assert.Len(t, fakeSAC.deleted, 0)
		}

		t.Log("Agent injected successfully, now testing cleanup and reinjection...")

		// Expect the agent to be reinjected.
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), "test-container-id").Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), "test-container-id", coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
		)

		// Terminate the agent and verify it is deleted.
		testutil.RequireSend(ctx, t, fakeDCCLI.execErrC, func(_ string, args ...string) error {
			if len(args) > 0 {
				assert.Equal(t, "agent", args[0])
			} else {
				assert.Fail(t, `want "agent" command argument`)
			}
			return errTestTermination
		})

		// Allow cleanup to proceed.
		testutil.RequireSend(ctx, t, fakeSAC.deleteErrC, nil)

		t.Log("Waiting for agent recreation...")

		// Allow agent recreation and reinjection to succeed.
		testutil.RequireSend(ctx, t, fakeSAC.createErrC, nil)
		testutil.RequireSend(ctx, t, fakeDCCLI.execErrC, func(cmd string, args ...string) error {
			assert.Equal(t, "pwd", cmd)
			assert.Empty(t, args)
			return nil
		}) // Exec pwd.

		// Wait until the agent recreation is started.
		for len(fakeSAC.createErrC) > 0 {
			_, aw := mClock.AdvanceNext()
			aw.MustWait(ctx)
		}

		t.Log("Agent recreated successfully.")

		// Verify agent was deleted.
		require.Len(t, fakeSAC.deleted, 1)
		assert.Equal(t, fakeSAC.created[0].ID, fakeSAC.deleted[0])

		// Verify the agent recreated.
		require.Len(t, fakeSAC.created, 2)
	})

	t.Run("SubAgentCleanup", func(t *testing.T) {
		t.Parallel()

		var (
			existingAgentID    = uuid.New()
			existingAgentToken = uuid.New()
			existingAgent      = agentcontainers.SubAgent{
				ID:        existingAgentID,
				Name:      "stopped-container",
				Directory: "/tmp",
				AuthToken: existingAgentToken,
			}

			ctx     = testutil.Context(t, testutil.WaitMedium)
			logger  = slog.Make()
			mClock  = quartz.NewMock(t)
			mCCLI   = acmock.NewMockContainerCLI(gomock.NewController(t))
			fakeSAC = &fakeSubAgentClient{
				agents: map[uuid.UUID]agentcontainers.SubAgent{
					existingAgentID: existingAgent,
				},
			}
		)

		mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{},
		}, nil).AnyTimes()

		mClock.Set(time.Now()).MustWait(ctx)
		tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithContainerCLI(mCCLI),
			agentcontainers.WithSubAgentClient(fakeSAC),
			agentcontainers.WithDevcontainerCLI(&fakeDevcontainerCLI{}),
		)
		defer api.Close()

		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		_, aw := mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify agent was deleted.
		assert.Contains(t, fakeSAC.deleted, existingAgentID)
		assert.Empty(t, fakeSAC.agents)
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		tests := []struct {
			name          string
			customization []agentcontainers.CoderCustomization
			afterCreate   func(t *testing.T, subAgent agentcontainers.SubAgent)
		}{
			{
				name:          "WithoutCustomization",
				customization: nil,
			},
			{
				name:          "WithDefaultDisplayApps",
				customization: []agentcontainers.CoderCustomization{},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Len(t, subAgent.DisplayApps, 4)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppVSCodeDesktop)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppWebTerminal)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppSSH)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppPortForward)
				},
			},
			{
				name: "WithAllDisplayApps",
				customization: []agentcontainers.CoderCustomization{
					{
						DisplayApps: map[codersdk.DisplayApp]bool{
							codersdk.DisplayAppSSH:            true,
							codersdk.DisplayAppWebTerminal:    true,
							codersdk.DisplayAppVSCodeDesktop:  true,
							codersdk.DisplayAppVSCodeInsiders: true,
							codersdk.DisplayAppPortForward:    true,
						},
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Len(t, subAgent.DisplayApps, 5)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppSSH)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppWebTerminal)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppVSCodeDesktop)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppVSCodeInsiders)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppPortForward)
				},
			},
			{
				name: "WithSomeDisplayAppsDisabled",
				customization: []agentcontainers.CoderCustomization{
					{
						DisplayApps: map[codersdk.DisplayApp]bool{
							codersdk.DisplayAppSSH:            false,
							codersdk.DisplayAppWebTerminal:    false,
							codersdk.DisplayAppVSCodeInsiders: false,

							// We'll enable vscode in this layer, and disable
							// it in the next layer to ensure a layer can be
							// disabled.
							codersdk.DisplayAppVSCodeDesktop: true,

							// We disable port-forward in this layer, and
							// then re-enable it in the next layer to ensure
							// that behavior works.
							codersdk.DisplayAppPortForward: false,
						},
					},
					{
						DisplayApps: map[codersdk.DisplayApp]bool{
							codersdk.DisplayAppVSCodeDesktop: false,
							codersdk.DisplayAppPortForward:   true,
						},
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Len(t, subAgent.DisplayApps, 1)
					assert.Contains(t, subAgent.DisplayApps, codersdk.DisplayAppPortForward)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var (
					ctx    = testutil.Context(t, testutil.WaitMedium)
					logger = testutil.Logger(t)
					mClock = quartz.NewMock(t)
					mCCLI  = acmock.NewMockContainerCLI(gomock.NewController(t))
					fSAC   = &fakeSubAgentClient{createErrC: make(chan error, 1)}
					fDCCLI = &fakeDevcontainerCLI{
						readConfig: agentcontainers.DevcontainerConfig{
							MergedConfiguration: agentcontainers.DevcontainerConfiguration{
								Customizations: agentcontainers.DevcontainerCustomizations{
									Coder: tt.customization,
								},
							},
						},
						execErrC: make(chan func(cmd string, args ...string) error, 1),
					}

					testContainer = codersdk.WorkspaceAgentContainer{
						ID:           "test-container-id",
						FriendlyName: "test-container",
						Image:        "test-image",
						Running:      true,
						CreatedAt:    time.Now(),
						Labels: map[string]string{
							agentcontainers.DevcontainerLocalFolderLabel: "/workspaces",
							agentcontainers.DevcontainerConfigFileLabel:  "/workspace/.devcontainer/devcontainer.json",
						},
					}
				)

				coderBin, err := os.Executable()
				require.NoError(t, err)

				// Mock the `List` function to always return out test container.
				mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
					Containers: []codersdk.WorkspaceAgentContainer{testContainer},
				}, nil).AnyTimes()

				// Mock the steps used for injecting the coder agent.
				gomock.InOrder(
					mCCLI.EXPECT().DetectArchitecture(gomock.Any(), testContainer.ID).Return(runtime.GOARCH, nil),
					mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
					mCCLI.EXPECT().Copy(gomock.Any(), testContainer.ID, coderBin, "/.coder-agent/coder").Return(nil),
					mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
				)

				mClock.Set(time.Now()).MustWait(ctx)
				tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

				api := agentcontainers.NewAPI(logger,
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(mCCLI),
					agentcontainers.WithDevcontainerCLI(fDCCLI),
					agentcontainers.WithSubAgentClient(fSAC),
					agentcontainers.WithSubAgentURL("test-subagent-url"),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				)
				defer api.Close()

				// Close before api.Close() defer to avoid deadlock after test.
				defer close(fSAC.createErrC)
				defer close(fDCCLI.execErrC)

				// Given: We allow agent creation and injection to succeed.
				testutil.RequireSend(ctx, t, fSAC.createErrC, nil)
				testutil.RequireSend(ctx, t, fDCCLI.execErrC, func(cmd string, args ...string) error {
					assert.Equal(t, "pwd", cmd)
					assert.Empty(t, args)
					return nil
				})

				// Wait until the ticker has been registered.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				// Then: We expected it to succeed
				require.Len(t, fSAC.created, 1)
				assert.Equal(t, testContainer.FriendlyName, fSAC.created[0].Name)

				if tt.afterCreate != nil {
					tt.afterCreate(t, fSAC.created[0])
				}
			})
		}
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

func fakeContainer(t *testing.T, mut ...func(*codersdk.WorkspaceAgentContainer)) codersdk.WorkspaceAgentContainer {
	t.Helper()
	ct := codersdk.WorkspaceAgentContainer{
		CreatedAt:    time.Now().UTC(),
		ID:           uuid.New().String(),
		FriendlyName: testutil.GetRandomName(t),
		Image:        testutil.GetRandomName(t) + ":" + strings.Split(uuid.New().String(), "-")[0],
		Labels: map[string]string{
			testutil.GetRandomName(t): testutil.GetRandomName(t),
		},
		Running: true,
		Ports: []codersdk.WorkspaceAgentContainerPort{
			{
				Network:  "tcp",
				Port:     testutil.RandomPortNoListen(t),
				HostPort: testutil.RandomPortNoListen(t),
				//nolint:gosec // this is a test
				HostIP: []string{"127.0.0.1", "[::1]", "localhost", "0.0.0.0", "[::]", testutil.GetRandomName(t)}[rand.Intn(6)],
			},
		},
		Status:  testutil.MustRandString(t, 10),
		Volumes: map[string]string{testutil.GetRandomName(t): testutil.GetRandomName(t)},
	}
	for _, m := range mut {
		m(&ct)
	}
	return ct
}
