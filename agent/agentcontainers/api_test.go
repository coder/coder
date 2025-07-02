package agentcontainers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentcontainers/acmock"
	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty"
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
	upErrC         chan func() error // If set, send to return err, close to return upErr.
	execErr        error
	execErrC       chan func(cmd string, args ...string) error // If set, send fn to return err, nil or close to return execErr.
	readConfig     agentcontainers.DevcontainerConfig
	readConfigErr  error
	readConfigErrC chan func(envs []string) error
}

func (f *fakeDevcontainerCLI) Up(ctx context.Context, _, _ string, _ ...agentcontainers.DevcontainerCLIUpOptions) (string, error) {
	if f.upErrC != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case fn, ok := <-f.upErrC:
			if ok {
				return f.upID, fn()
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

func (f *fakeDevcontainerCLI) ReadConfig(ctx context.Context, _, _ string, envs []string, _ ...agentcontainers.DevcontainerCLIReadConfigOptions) (agentcontainers.DevcontainerConfig, error) {
	if f.readConfigErrC != nil {
		select {
		case <-ctx.Done():
			return agentcontainers.DevcontainerConfig{}, ctx.Err()
		case fn, ok := <-f.readConfigErrC:
			if ok {
				return f.readConfig, fn(envs)
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
		return nil, watcher.ErrClosed
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
	logger slog.Logger
	agents map[uuid.UUID]agentcontainers.SubAgent

	listErrC   chan error // If set, send to return error, close to return nil.
	created    []agentcontainers.SubAgent
	createErrC chan error // If set, send to return error, close to return nil.
	deleted    []uuid.UUID
	deleteErrC chan error // If set, send to return error, close to return nil.
}

func (m *fakeSubAgentClient) List(ctx context.Context) ([]agentcontainers.SubAgent, error) {
	if m.listErrC != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-m.listErrC:
			if err != nil {
				return nil, err
			}
		}
	}
	var agents []agentcontainers.SubAgent
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}

func (m *fakeSubAgentClient) Create(ctx context.Context, agent agentcontainers.SubAgent) (agentcontainers.SubAgent, error) {
	m.logger.Debug(ctx, "creating sub agent", slog.F("agent", agent))
	if m.createErrC != nil {
		select {
		case <-ctx.Done():
			return agentcontainers.SubAgent{}, ctx.Err()
		case err := <-m.createErrC:
			if err != nil {
				return agentcontainers.SubAgent{}, err
			}
		}
	}
	if agent.Name == "" {
		return agentcontainers.SubAgent{}, xerrors.New("name must be set")
	}
	if agent.Architecture == "" {
		return agentcontainers.SubAgent{}, xerrors.New("architecture must be set")
	}
	if agent.OperatingSystem == "" {
		return agentcontainers.SubAgent{}, xerrors.New("operating system must be set")
	}

	for _, a := range m.agents {
		if a.Name == agent.Name {
			return agentcontainers.SubAgent{}, &pq.Error{
				Code:    "23505",
				Message: fmt.Sprintf("workspace agent name %q already exists in this workspace build", agent.Name),
			}
		}
	}

	agent.ID = uuid.New()
	agent.AuthToken = uuid.New()
	if m.agents == nil {
		m.agents = make(map[uuid.UUID]agentcontainers.SubAgent)
	}
	m.agents[agent.ID] = agent
	m.created = append(m.created, agent)
	return agent, nil
}

func (m *fakeSubAgentClient) Delete(ctx context.Context, id uuid.UUID) error {
	m.logger.Debug(ctx, "deleting sub agent", slog.F("id", id.String()))
	if m.deleteErrC != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-m.deleteErrC:
			if err != nil {
				return err
			}
		}
	}
	if m.agents == nil {
		m.agents = make(map[uuid.UUID]agentcontainers.SubAgent)
	}
	delete(m.agents, id)
	m.deleted = append(m.deleted, id)
	return nil
}

// fakeExecer implements agentexec.Execer for testing and tracks execution details.
type fakeExecer struct {
	commands        [][]string
	createdCommands []*exec.Cmd
}

func (f *fakeExecer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	f.commands = append(f.commands, append([]string{cmd}, args...))
	// Create a command that returns empty JSON for docker commands.
	c := exec.CommandContext(ctx, "echo", "[]")
	f.createdCommands = append(f.createdCommands, c)
	return c
}

func (f *fakeExecer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	f.commands = append(f.commands, append([]string{cmd}, args...))
	return &pty.Cmd{
		Context: ctx,
		Path:    cmd,
		Args:    append([]string{cmd}, args...),
		Env:     []string{},
		Dir:     "",
	}
}

func (f *fakeExecer) getLastCommand() *exec.Cmd {
	if len(f.createdCommands) == 0 {
		return nil
	}
	return f.createdCommands[len(f.createdCommands)-1]
}

func TestAPI(t *testing.T) {
	t.Parallel()

	t.Run("NoUpdaterLoopLogspam", func(t *testing.T) {
		t.Parallel()

		var (
			ctx        = testutil.Context(t, testutil.WaitShort)
			logbuf     strings.Builder
			logger     = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).AppendSinks(sloghuman.Sink(&logbuf))
			mClock     = quartz.NewMock(t)
			tickerTrap = mClock.Trap().TickerFunc("updaterLoop")
			firstErr   = xerrors.New("first error")
			secondErr  = xerrors.New("second error")
			fakeCLI    = &fakeContainerCLI{
				listErr: firstErr,
			}
		)

		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithContainerCLI(fakeCLI),
		)
		api.Start()
		defer api.Close()

		// Make sure the ticker function has been registered
		// before advancing the clock.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		logbuf.Reset()

		// First tick should handle the error.
		_, aw := mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify first error is logged.
		got := logbuf.String()
		t.Logf("got log: %q", got)
		require.Contains(t, got, "updater loop ticker failed", "first error should be logged")
		require.Contains(t, got, "first error", "should contain first error message")
		logbuf.Reset()

		// Second tick should handle the same error without logging it again.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify same error is not logged again.
		got = logbuf.String()
		t.Logf("got log: %q", got)
		require.Empty(t, got, "same error should not be logged again")

		// Change to a different error.
		fakeCLI.listErr = secondErr

		// Third tick should handle the different error and log it.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify different error is logged.
		got = logbuf.String()
		t.Logf("got log: %q", got)
		require.Contains(t, got, "updater loop ticker failed", "different error should be logged")
		require.Contains(t, got, "second error", "should contain second error message")
		logbuf.Reset()

		// Clear the error to simulate success.
		fakeCLI.listErr = nil

		// Fourth tick should succeed.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Fifth tick should continue to succeed.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify successful operations are logged properly.
		got = logbuf.String()
		t.Logf("got log: %q", got)
		gotSuccessCount := strings.Count(got, "containers updated successfully")
		require.GreaterOrEqual(t, gotSuccessCount, 2, "should have successful update got")
		require.NotContains(t, got, "updater loop ticker failed", "no errors should be logged during success")
		logbuf.Reset()

		// Reintroduce the original error.
		fakeCLI.listErr = firstErr

		// Sixth tick should handle the error after success and log it.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify error after success is logged.
		got = logbuf.String()
		t.Logf("got log: %q", got)
		require.Contains(t, got, "updater loop ticker failed", "error after success should be logged")
		require.Contains(t, got, "first error", "should contain first error message")
		logbuf.Reset()
	})

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
				api.Start()
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

		devcontainerID1 := uuid.New()
		devcontainerID2 := uuid.New()
		workspaceFolder1 := "/workspace/test1"
		workspaceFolder2 := "/workspace/test2"
		configPath1 := "/workspace/test1/.devcontainer/devcontainer.json"
		configPath2 := "/workspace/test2/.devcontainer/devcontainer.json"

		// Create a container that represents an existing devcontainer
		devContainer1 := codersdk.WorkspaceAgentContainer{
			ID:           "container-1",
			FriendlyName: "test-container-1",
			Running:      true,
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: workspaceFolder1,
				agentcontainers.DevcontainerConfigFileLabel:  configPath1,
			},
		}

		devContainer2 := codersdk.WorkspaceAgentContainer{
			ID:           "container-2",
			FriendlyName: "test-container-2",
			Running:      true,
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: workspaceFolder2,
				agentcontainers.DevcontainerConfigFileLabel:  configPath2,
			},
		}

		tests := []struct {
			name               string
			devcontainerID     string
			setupDevcontainers []codersdk.WorkspaceAgentDevcontainer
			lister             *fakeContainerCLI
			devcontainerCLI    *fakeDevcontainerCLI
			wantStatus         []int
			wantBody           []string
		}{
			{
				name:            "Missing devcontainer ID",
				devcontainerID:  "",
				lister:          &fakeContainerCLI{},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusBadRequest},
				wantBody:        []string{"Missing devcontainer ID"},
			},
			{
				name:           "Devcontainer not found",
				devcontainerID: uuid.NewString(),
				lister: &fakeContainerCLI{
					arch: "<none>", // Unsupported architecture, don't inject subagent.
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      []int{http.StatusNotFound},
				wantBody:        []string{"Devcontainer not found"},
			},
			{
				name:           "Devcontainer CLI error",
				devcontainerID: devcontainerID1.String(),
				setupDevcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              devcontainerID1,
						Name:            "test-devcontainer-1",
						WorkspaceFolder: workspaceFolder1,
						ConfigPath:      configPath1,
						Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
						Container:       &devContainer1,
					},
				},
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{devContainer1},
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
				name:           "OK",
				devcontainerID: devcontainerID2.String(),
				setupDevcontainers: []codersdk.WorkspaceAgentDevcontainer{
					{
						ID:              devcontainerID2,
						Name:            "test-devcontainer-2",
						WorkspaceFolder: workspaceFolder2,
						ConfigPath:      configPath2,
						Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
						Container:       &devContainer2,
					},
				},
				lister: &fakeContainerCLI{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{devContainer2},
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

				tt.devcontainerCLI.upErrC = make(chan func() error)

				// Setup router with the handler under test.
				r := chi.NewRouter()

				api := agentcontainers.NewAPI(
					logger,
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(tt.lister),
					agentcontainers.WithDevcontainerCLI(tt.devcontainerCLI),
					agentcontainers.WithWatcher(watcher.NewNoop()),
					agentcontainers.WithDevcontainers(tt.setupDevcontainers, nil),
				)

				api.Start()
				defer api.Close()
				r.Mount("/", api.Routes())

				// Make sure the ticker function has been registered
				// before advancing the clock.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				for i := range tt.wantStatus {
					// Simulate HTTP request to the recreate endpoint.
					req := httptest.NewRequest(http.MethodPost, "/devcontainers/"+tt.devcontainerID+"/recreate", nil).
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
				req := httptest.NewRequest(http.MethodGet, "/", nil).
					WithContext(ctx)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				require.Equal(t, http.StatusOK, rec.Code, "status code mismatch")
				var resp codersdk.WorkspaceAgentListContainersResponse
				t.Log(rec.Body.String())
				err := json.NewDecoder(rec.Body).Decode(&resp)
				require.NoError(t, err, "unmarshal response failed")
				require.Len(t, resp.Devcontainers, 1, "expected one devcontainer in response")
				assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusStarting, resp.Devcontainers[0].Status, "devcontainer is not starting")
				require.NotNil(t, resp.Devcontainers[0].Container, "devcontainer should have container reference")

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

					req = httptest.NewRequest(http.MethodGet, "/", nil).
						WithContext(ctx)
					rec = httptest.NewRecorder()
					r.ServeHTTP(rec, req)

					require.Equal(t, http.StatusOK, rec.Code, "status code mismatch after error")
					err = json.NewDecoder(rec.Body).Decode(&resp)
					require.NoError(t, err, "unmarshal response failed after error")
					require.Len(t, resp.Devcontainers, 1, "expected one devcontainer in response after error")
					assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusError, resp.Devcontainers[0].Status, "devcontainer is not in an error state after up failure")
					require.NotNil(t, resp.Devcontainers[0].Container, "devcontainer should have container reference after up failure")
					return
				}

				// Ensure the devcontainer ends up in success state.
				nowRecreateSuccessTrap.MustWait(ctx).MustRelease(ctx)
				nowRecreateSuccessTrap.Close()

				// Advance the clock to run the devcontainer state update routine.
				_, aw = mClock.AdvanceNext()
				aw.MustWait(ctx)

				req = httptest.NewRequest(http.MethodGet, "/", nil).
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
			wantTestContainer  bool
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
					require.NotNil(t, runtime1.Container)
					assert.Equal(t, "runtime-container-1", runtime1.Container.ID)
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

					require.NotNil(t, nonRunning.Container, "non-running container should have container reference")
					assert.Equal(t, "non-running-container", nonRunning.Container.ID)
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
									agentcontainers.DevcontainerLocalFolderLabel: "/workspace/project1",
									agentcontainers.DevcontainerConfigFileLabel:  "/workspace/project1/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "project2-container",
								FriendlyName: "project2-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/home/user/project2",
									agentcontainers.DevcontainerConfigFileLabel:  "/home/user/project2/.devcontainer/devcontainer.json",
								},
							},
							{
								ID:           "project3-container",
								FriendlyName: "project3-container",
								Running:      true,
								Labels: map[string]string{
									agentcontainers.DevcontainerLocalFolderLabel: "/var/lib/project3",
									agentcontainers.DevcontainerConfigFileLabel:  "/var/lib/project3/.devcontainer/devcontainer.json",
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
			{
				name:              "Include test containers",
				lister:            &fakeContainerCLI{},
				wantStatus:        http.StatusOK,
				wantTestContainer: true,
				wantCount:         1, // Will be appended.
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

				mClock := quartz.NewMock(t)
				mClock.Set(time.Now()).MustWait(testutil.Context(t, testutil.WaitShort))
				tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

				// This container should be ignored unless explicitly included.
				tt.lister.containers.Containers = append(tt.lister.containers.Containers, codersdk.WorkspaceAgentContainer{
					ID:           "test-container-1",
					FriendlyName: "test-container-1",
					Running:      true,
					Labels: map[string]string{
						agentcontainers.DevcontainerLocalFolderLabel: "/workspace/test1",
						agentcontainers.DevcontainerConfigFileLabel:  "/workspace/test1/.devcontainer/devcontainer.json",
						agentcontainers.DevcontainerIsTestRunLabel:   "true",
					},
				})

				// Setup router with the handler under test.
				r := chi.NewRouter()
				apiOptions := []agentcontainers.Option{
					agentcontainers.WithClock(mClock),
					agentcontainers.WithContainerCLI(tt.lister),
					agentcontainers.WithDevcontainerCLI(&fakeDevcontainerCLI{}),
					agentcontainers.WithWatcher(watcher.NewNoop()),
				}

				if tt.wantTestContainer {
					apiOptions = append(apiOptions, agentcontainers.WithContainerLabelIncludeFilter(
						agentcontainers.DevcontainerIsTestRunLabel, "true",
					))
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
				api.Start()
				defer api.Close()

				r.Mount("/", api.Routes())

				ctx := testutil.Context(t, testutil.WaitShort)

				// Make sure the ticker function has been registered
				// before advancing the clock.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				for _, dc := range tt.knownDevcontainers {
					err := api.CreateDevcontainer(dc.WorkspaceFolder, dc.ConfigPath)
					require.NoError(t, err)
				}

				// Advance the clock to run the updater loop.
				_, aw := mClock.AdvanceNext()
				aw.MustWait(ctx)

				req := httptest.NewRequest(http.MethodGet, "/", nil).
					WithContext(ctx)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				// Check the response status code.
				require.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
				if tt.wantStatus != http.StatusOK {
					return
				}

				var response codersdk.WorkspaceAgentListContainersResponse
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
		api.Start()
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
		req := httptest.NewRequest(http.MethodGet, "/", nil).
			WithContext(ctx)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp1 codersdk.WorkspaceAgentListContainersResponse
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
		req = httptest.NewRequest(http.MethodGet, "/", nil).
			WithContext(ctx)
		rec = httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp2 codersdk.WorkspaceAgentListContainersResponse
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
		api.Start()
		defer api.Close()

		r := chi.NewRouter()
		r.Mount("/", api.Routes())

		// Make sure the ticker function has been registered
		// before advancing any use of mClock.Advance.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Call the list endpoint first to ensure config files are
		// detected and watched.
		req := httptest.NewRequest(http.MethodGet, "/", nil).
			WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var response codersdk.WorkspaceAgentListContainersResponse
		err := json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)
		require.Len(t, response.Devcontainers, 1)
		assert.False(t, response.Devcontainers[0].Dirty,
			"devcontainer should not be marked as dirty initially")
		assert.Equal(t, codersdk.WorkspaceAgentDevcontainerStatusRunning, response.Devcontainers[0].Status, "devcontainer should be running initially")
		require.NotNil(t, response.Devcontainers[0].Container, "container should not be nil")

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
		req = httptest.NewRequest(http.MethodGet, "/", nil).
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

		container.ID = "new-container-id" // Simulate a new container ID after recreation.
		container.FriendlyName = "new-container-name"
		container.CreatedAt = mClock.Now() // Update the creation time.
		fLister.containers.Containers = []codersdk.WorkspaceAgentContainer{container}

		// Advance the clock to run updaterLoop.
		_, aw = mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Check if dirty flag is cleared.
		req = httptest.NewRequest(http.MethodGet, "/", nil).
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
				logger:     logger.Named("fakeSubAgentClient"),
				createErrC: make(chan error, 1),
				deleteErrC: make(chan error, 1),
			}
			fakeDCCLI = &fakeDevcontainerCLI{
				readConfig: agentcontainers.DevcontainerConfig{
					Workspace: agentcontainers.DevcontainerWorkspace{
						WorkspaceFolder: "/workspaces/coder",
					},
				},
				execErrC:       make(chan func(cmd string, args ...string) error, 1),
				readConfigErrC: make(chan func(envs []string) error, 1),
			}

			testContainer = codersdk.WorkspaceAgentContainer{
				ID:           "test-container-id",
				FriendlyName: "test-container",
				Image:        "test-image",
				Running:      true,
				CreatedAt:    time.Now(),
				Labels: map[string]string{
					agentcontainers.DevcontainerLocalFolderLabel: "/home/coder/coder",
					agentcontainers.DevcontainerConfigFileLabel:  "/home/coder/coder/.devcontainer/devcontainer.json",
				},
			}
		)

		coderBin, err := os.Executable()
		require.NoError(t, err)

		mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{testContainer},
		}, nil).Times(3) // 1 initial call + 2 updates.
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), "test-container-id").Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), "test-container-id", coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
		)

		mClock.Set(time.Now()).MustWait(ctx)
		tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

		var closeOnce sync.Once
		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithContainerCLI(mCCLI),
			agentcontainers.WithWatcher(watcher.NewNoop()),
			agentcontainers.WithSubAgentClient(fakeSAC),
			agentcontainers.WithSubAgentURL("test-subagent-url"),
			agentcontainers.WithDevcontainerCLI(fakeDCCLI),
			agentcontainers.WithManifestInfo("test-user", "test-workspace", "test-parent-agent"),
		)
		api.Start()
		apiClose := func() {
			closeOnce.Do(func() {
				// Close before api.Close() defer to avoid deadlock after test.
				close(fakeSAC.createErrC)
				close(fakeSAC.deleteErrC)
				close(fakeDCCLI.execErrC)
				close(fakeDCCLI.readConfigErrC)

				_ = api.Close()
			})
		}
		defer apiClose()

		// Allow initial agent creation and injection to succeed.
		testutil.RequireSend(ctx, t, fakeSAC.createErrC, nil)
		testutil.RequireSend(ctx, t, fakeDCCLI.readConfigErrC, func(envs []string) error {
			assert.Contains(t, envs, "CODER_WORKSPACE_AGENT_NAME=coder")
			assert.Contains(t, envs, "CODER_WORKSPACE_NAME=test-workspace")
			assert.Contains(t, envs, "CODER_WORKSPACE_OWNER_NAME=test-user")
			assert.Contains(t, envs, "CODER_WORKSPACE_PARENT_AGENT_NAME=test-parent-agent")
			assert.Contains(t, envs, "CODER_URL=test-subagent-url")
			assert.Contains(t, envs, "CONTAINER_ID=test-container-id")
			return nil
		})

		// Make sure the ticker function has been registered
		// before advancing the clock.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Refresh twice to ensure idempotency of agent creation.
		err = api.RefreshContainers(ctx)
		require.NoError(t, err, "refresh containers should not fail")
		t.Logf("Agents created: %d, deleted: %d", len(fakeSAC.created), len(fakeSAC.deleted))

		err = api.RefreshContainers(ctx)
		require.NoError(t, err, "refresh containers should not fail")
		t.Logf("Agents created: %d, deleted: %d", len(fakeSAC.created), len(fakeSAC.deleted))

		// Verify agent was created.
		require.Len(t, fakeSAC.created, 1)
		assert.Equal(t, "coder", fakeSAC.created[0].Name)
		assert.Equal(t, "/workspaces/coder", fakeSAC.created[0].Directory)
		assert.Len(t, fakeSAC.deleted, 0)

		t.Log("Agent injected successfully, now testing reinjection into the same container...")

		// Terminate the agent and verify it can be reinjected.
		terminated := make(chan struct{})
		testutil.RequireSend(ctx, t, fakeDCCLI.execErrC, func(_ string, args ...string) error {
			defer close(terminated)
			if len(args) > 0 {
				assert.Equal(t, "agent", args[0])
			} else {
				assert.Fail(t, `want "agent" command argument`)
			}
			return errTestTermination
		})
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for agent termination")
		case <-terminated:
		}

		t.Log("Waiting for agent reinjection...")

		// Expect the agent to be reinjected.
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), "test-container-id").Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), "test-container-id", coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "test-container-id", "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
		)

		// Verify that the agent has started.
		agentStarted := make(chan struct{})
		continueTerminate := make(chan struct{})
		terminated = make(chan struct{})
		testutil.RequireSend(ctx, t, fakeDCCLI.execErrC, func(_ string, args ...string) error {
			defer close(terminated)
			if len(args) > 0 {
				assert.Equal(t, "agent", args[0])
			} else {
				assert.Fail(t, `want "agent" command argument`)
			}
			close(agentStarted)
			select {
			case <-ctx.Done():
				t.Error("timeout waiting for agent continueTerminate")
			case <-continueTerminate:
			}
			return errTestTermination
		})

	WaitStartLoop:
		for {
			// Agent reinjection will succeed and we will not re-create the
			// agent.
			mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{testContainer},
			}, nil).Times(1) // 1 update.
			err = api.RefreshContainers(ctx)
			require.NoError(t, err, "refresh containers should not fail")

			t.Logf("Agents created: %d, deleted: %d", len(fakeSAC.created), len(fakeSAC.deleted))

			select {
			case <-agentStarted:
				break WaitStartLoop
			case <-ctx.Done():
				t.Fatal("timeout waiting for agent to start")
			default:
			}
		}

		// Verify that the agent was reused.
		require.Len(t, fakeSAC.created, 1)
		assert.Len(t, fakeSAC.deleted, 0)

		t.Log("Agent reinjected successfully, now testing agent deletion and recreation...")

		// New container ID means the agent will be recreated.
		testContainer.ID = "new-test-container-id" // Simulate a new container ID after recreation.
		// Expect the agent to be injected.
		mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{testContainer},
		}, nil).Times(1) // 1 update.
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), "new-test-container-id").Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "new-test-container-id", "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), "new-test-container-id", coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "new-test-container-id", "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), "new-test-container-id", "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
		)

		fakeDCCLI.readConfig.MergedConfiguration.Customizations.Coder = []agentcontainers.CoderCustomization{
			{
				DisplayApps: map[codersdk.DisplayApp]bool{
					codersdk.DisplayAppSSH:            true,
					codersdk.DisplayAppWebTerminal:    true,
					codersdk.DisplayAppVSCodeDesktop:  true,
					codersdk.DisplayAppVSCodeInsiders: true,
					codersdk.DisplayAppPortForward:    true,
				},
			},
		}

		// Terminate the running agent.
		close(continueTerminate)
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for agent termination")
		case <-terminated:
		}

		// Simulate the agent deletion (this happens because the
		// devcontainer configuration changed).
		testutil.RequireSend(ctx, t, fakeSAC.deleteErrC, nil)
		// Expect the agent to be recreated.
		testutil.RequireSend(ctx, t, fakeSAC.createErrC, nil)
		testutil.RequireSend(ctx, t, fakeDCCLI.readConfigErrC, func(envs []string) error {
			assert.Contains(t, envs, "CODER_WORKSPACE_AGENT_NAME=coder")
			assert.Contains(t, envs, "CODER_WORKSPACE_NAME=test-workspace")
			assert.Contains(t, envs, "CODER_WORKSPACE_OWNER_NAME=test-user")
			assert.Contains(t, envs, "CODER_WORKSPACE_PARENT_AGENT_NAME=test-parent-agent")
			assert.Contains(t, envs, "CODER_URL=test-subagent-url")
			assert.NotContains(t, envs, "CONTAINER_ID=test-container-id")
			return nil
		})

		err = api.RefreshContainers(ctx)
		require.NoError(t, err, "refresh containers should not fail")
		t.Logf("Agents created: %d, deleted: %d", len(fakeSAC.created), len(fakeSAC.deleted))

		// Verify the agent was deleted and recreated.
		require.Len(t, fakeSAC.deleted, 1, "there should be one deleted agent after recreation")
		assert.Len(t, fakeSAC.created, 2, "there should be two created agents after recreation")
		assert.Equal(t, fakeSAC.created[0].ID, fakeSAC.deleted[0], "the deleted agent should match the first created agent")

		t.Log("Agent deleted and recreated successfully.")

		apiClose()
		require.Len(t, fakeSAC.created, 2, "API close should not create more agents")
		require.Len(t, fakeSAC.deleted, 2, "API close should delete the agent")
		assert.Equal(t, fakeSAC.created[1].ID, fakeSAC.deleted[1], "the second created agent should be deleted on API close")
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
				logger: logger.Named("fakeSubAgentClient"),
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
		api.Start()
		defer api.Close()

		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		_, aw := mClock.AdvanceNext()
		aw.MustWait(ctx)

		// Verify agent was deleted.
		assert.Contains(t, fakeSAC.deleted, existingAgentID)
		assert.Empty(t, fakeSAC.agents)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		t.Run("DuringUp", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitMedium)
				logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				mClock = quartz.NewMock(t)
				fCCLI  = &fakeContainerCLI{arch: "<none>"}
				fDCCLI = &fakeDevcontainerCLI{
					upErrC: make(chan func() error, 1),
				}
				fSAC = &fakeSubAgentClient{
					logger: logger.Named("fakeSubAgentClient"),
				}

				testDevcontainer = codersdk.WorkspaceAgentDevcontainer{
					ID:              uuid.New(),
					Name:            "test-devcontainer",
					WorkspaceFolder: "/workspaces/project",
					ConfigPath:      "/workspaces/project/.devcontainer/devcontainer.json",
					Status:          codersdk.WorkspaceAgentDevcontainerStatusStopped,
				}
			)

			mClock.Set(time.Now()).MustWait(ctx)
			tickerTrap := mClock.Trap().TickerFunc("updaterLoop")
			nowRecreateErrorTrap := mClock.Trap().Now("recreate", "errorTimes")
			nowRecreateSuccessTrap := mClock.Trap().Now("recreate", "successTimes")

			api := agentcontainers.NewAPI(logger,
				agentcontainers.WithClock(mClock),
				agentcontainers.WithContainerCLI(fCCLI),
				agentcontainers.WithDevcontainerCLI(fDCCLI),
				agentcontainers.WithDevcontainers(
					[]codersdk.WorkspaceAgentDevcontainer{testDevcontainer},
					[]codersdk.WorkspaceAgentScript{{ID: testDevcontainer.ID, LogSourceID: uuid.New()}},
				),
				agentcontainers.WithSubAgentClient(fSAC),
				agentcontainers.WithSubAgentURL("test-subagent-url"),
				agentcontainers.WithWatcher(watcher.NewNoop()),
			)
			api.Start()
			defer func() {
				close(fDCCLI.upErrC)
				api.Close()
			}()

			r := chi.NewRouter()
			r.Mount("/", api.Routes())

			tickerTrap.MustWait(ctx).MustRelease(ctx)
			tickerTrap.Close()

			// Given: We send a 'recreate' request.
			req := httptest.NewRequest(http.MethodPost, "/devcontainers/"+testDevcontainer.ID.String()+"/recreate", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusAccepted, rec.Code)

			// Given: We simulate an error running `devcontainer up`
			simulatedError := xerrors.New("simulated error")
			testutil.RequireSend(ctx, t, fDCCLI.upErrC, func() error { return simulatedError })

			nowRecreateErrorTrap.MustWait(ctx).MustRelease(ctx)
			nowRecreateErrorTrap.Close()

			req = httptest.NewRequest(http.MethodGet, "/", nil)
			rec = httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			var response codersdk.WorkspaceAgentListContainersResponse
			err := json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)

			// Then: We expect that there will be an error associated with the devcontainer.
			require.Len(t, response.Devcontainers, 1)
			require.Equal(t, "simulated error", response.Devcontainers[0].Error)

			// Given: We send another 'recreate' request.
			req = httptest.NewRequest(http.MethodPost, "/devcontainers/"+testDevcontainer.ID.String()+"/recreate", nil)
			rec = httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusAccepted, rec.Code)

			// Given: We allow `devcontainer up` to succeed.
			testutil.RequireSend(ctx, t, fDCCLI.upErrC, func() error {
				req = httptest.NewRequest(http.MethodGet, "/", nil)
				rec = httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code)

				response = codersdk.WorkspaceAgentListContainersResponse{}
				err = json.NewDecoder(rec.Body).Decode(&response)
				require.NoError(t, err)

				// Then: We make sure that the error has been cleared before running up.
				require.Len(t, response.Devcontainers, 1)
				require.Equal(t, "", response.Devcontainers[0].Error)

				return nil
			})

			nowRecreateSuccessTrap.MustWait(ctx).MustRelease(ctx)
			nowRecreateSuccessTrap.Close()

			req = httptest.NewRequest(http.MethodGet, "/", nil)
			rec = httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			response = codersdk.WorkspaceAgentListContainersResponse{}
			err = json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)

			// Then: We also expect no error after running up..
			require.Len(t, response.Devcontainers, 1)
			require.Equal(t, "", response.Devcontainers[0].Error)
		})

		t.Run("DuringInjection", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitMedium)
				logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				mClock = quartz.NewMock(t)
				mCCLI  = acmock.NewMockContainerCLI(gomock.NewController(t))
				fDCCLI = &fakeDevcontainerCLI{}
				fSAC   = &fakeSubAgentClient{
					logger:     logger.Named("fakeSubAgentClient"),
					createErrC: make(chan error, 1),
				}

				containerCreatedAt = time.Now()
				testContainer      = codersdk.WorkspaceAgentContainer{
					ID:           "test-container-id",
					FriendlyName: "test-container",
					Image:        "test-image",
					Running:      true,
					CreatedAt:    containerCreatedAt,
					Labels: map[string]string{
						agentcontainers.DevcontainerLocalFolderLabel: "/workspaces",
						agentcontainers.DevcontainerConfigFileLabel:  "/workspace/.devcontainer/devcontainer.json",
					},
				}
			)

			coderBin, err := os.Executable()
			require.NoError(t, err)

			// Mock the `List` function to always return the test container.
			mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{testContainer},
			}, nil).AnyTimes()

			// We're going to force the container CLI to fail, which will allow us to test the
			// error handling.
			simulatedError := xerrors.New("simulated error")
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), testContainer.ID).Return("", simulatedError).Times(1)

			mClock.Set(containerCreatedAt).MustWait(ctx)
			tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

			api := agentcontainers.NewAPI(logger,
				agentcontainers.WithClock(mClock),
				agentcontainers.WithContainerCLI(mCCLI),
				agentcontainers.WithDevcontainerCLI(fDCCLI),
				agentcontainers.WithSubAgentClient(fSAC),
				agentcontainers.WithSubAgentURL("test-subagent-url"),
				agentcontainers.WithWatcher(watcher.NewNoop()),
			)
			api.Start()
			defer func() {
				close(fSAC.createErrC)
				api.Close()
			}()

			r := chi.NewRouter()
			r.Mount("/", api.Routes())

			// Given: We allow an attempt at creation to occur.
			tickerTrap.MustWait(ctx).MustRelease(ctx)
			tickerTrap.Close()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			var response codersdk.WorkspaceAgentListContainersResponse
			err = json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)

			// Then: We expect that there will be an error associated with the devcontainer.
			require.Len(t, response.Devcontainers, 1)
			require.Equal(t, "detect architecture: simulated error", response.Devcontainers[0].Error)

			gomock.InOrder(
				mCCLI.EXPECT().DetectArchitecture(gomock.Any(), testContainer.ID).Return(runtime.GOARCH, nil),
				mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
				mCCLI.EXPECT().Copy(gomock.Any(), testContainer.ID, coderBin, "/.coder-agent/coder").Return(nil),
				mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
				mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
			)

			// Given: We allow creation to succeed.
			testutil.RequireSend(ctx, t, fSAC.createErrC, nil)

			_, aw := mClock.AdvanceNext()
			aw.MustWait(ctx)

			req = httptest.NewRequest(http.MethodGet, "/", nil)
			rec = httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)

			response = codersdk.WorkspaceAgentListContainersResponse{}
			err = json.NewDecoder(rec.Body).Decode(&response)
			require.NoError(t, err)

			// Then: We expect that the error will be gone
			require.Len(t, response.Devcontainers, 1)
			require.Equal(t, "", response.Devcontainers[0].Error)
		})
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		tests := []struct {
			name                 string
			customization        agentcontainers.CoderCustomization
			mergedCustomizations []agentcontainers.CoderCustomization
			afterCreate          func(t *testing.T, subAgent agentcontainers.SubAgent)
		}{
			{
				name:                 "WithoutCustomization",
				mergedCustomizations: nil,
			},
			{
				name:                 "WithDefaultDisplayApps",
				mergedCustomizations: []agentcontainers.CoderCustomization{},
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
				mergedCustomizations: []agentcontainers.CoderCustomization{
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
				mergedCustomizations: []agentcontainers.CoderCustomization{
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
			{
				name: "WithApps",
				mergedCustomizations: []agentcontainers.CoderCustomization{
					{
						Apps: []agentcontainers.SubAgentApp{
							{
								Slug:        "web-app",
								DisplayName: "Web Application",
								URL:         "http://localhost:8080",
								OpenIn:      codersdk.WorkspaceAppOpenInTab,
								Share:       codersdk.WorkspaceAppSharingLevelOwner,
								Icon:        "/icons/web.svg",
								Order:       int32(1),
							},
							{
								Slug:        "api-server",
								DisplayName: "API Server",
								URL:         "http://localhost:3000",
								OpenIn:      codersdk.WorkspaceAppOpenInSlimWindow,
								Share:       codersdk.WorkspaceAppSharingLevelAuthenticated,
								Icon:        "/icons/api.svg",
								Order:       int32(2),
								Hidden:      true,
							},
							{
								Slug:        "docs",
								DisplayName: "Documentation",
								URL:         "http://localhost:4000",
								OpenIn:      codersdk.WorkspaceAppOpenInTab,
								Share:       codersdk.WorkspaceAppSharingLevelPublic,
								Icon:        "/icons/book.svg",
								Order:       int32(3),
							},
						},
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Len(t, subAgent.Apps, 3)

					// Verify first app
					assert.Equal(t, "web-app", subAgent.Apps[0].Slug)
					assert.Equal(t, "Web Application", subAgent.Apps[0].DisplayName)
					assert.Equal(t, "http://localhost:8080", subAgent.Apps[0].URL)
					assert.Equal(t, codersdk.WorkspaceAppOpenInTab, subAgent.Apps[0].OpenIn)
					assert.Equal(t, codersdk.WorkspaceAppSharingLevelOwner, subAgent.Apps[0].Share)
					assert.Equal(t, "/icons/web.svg", subAgent.Apps[0].Icon)
					assert.Equal(t, int32(1), subAgent.Apps[0].Order)

					// Verify second app
					assert.Equal(t, "api-server", subAgent.Apps[1].Slug)
					assert.Equal(t, "API Server", subAgent.Apps[1].DisplayName)
					assert.Equal(t, "http://localhost:3000", subAgent.Apps[1].URL)
					assert.Equal(t, codersdk.WorkspaceAppOpenInSlimWindow, subAgent.Apps[1].OpenIn)
					assert.Equal(t, codersdk.WorkspaceAppSharingLevelAuthenticated, subAgent.Apps[1].Share)
					assert.Equal(t, "/icons/api.svg", subAgent.Apps[1].Icon)
					assert.Equal(t, int32(2), subAgent.Apps[1].Order)
					assert.Equal(t, true, subAgent.Apps[1].Hidden)

					// Verify third app
					assert.Equal(t, "docs", subAgent.Apps[2].Slug)
					assert.Equal(t, "Documentation", subAgent.Apps[2].DisplayName)
					assert.Equal(t, "http://localhost:4000", subAgent.Apps[2].URL)
					assert.Equal(t, codersdk.WorkspaceAppOpenInTab, subAgent.Apps[2].OpenIn)
					assert.Equal(t, codersdk.WorkspaceAppSharingLevelPublic, subAgent.Apps[2].Share)
					assert.Equal(t, "/icons/book.svg", subAgent.Apps[2].Icon)
					assert.Equal(t, int32(3), subAgent.Apps[2].Order)
				},
			},
			{
				name: "AppDeduplication",
				mergedCustomizations: []agentcontainers.CoderCustomization{
					{
						Apps: []agentcontainers.SubAgentApp{
							{
								Slug:   "foo-app",
								Hidden: true,
								Order:  1,
							},
							{
								Slug: "bar-app",
							},
						},
					},
					{
						Apps: []agentcontainers.SubAgentApp{
							{
								Slug:  "foo-app",
								Order: 2,
							},
							{
								Slug: "baz-app",
							},
						},
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Len(t, subAgent.Apps, 3)

					// As the original "foo-app" gets overridden by the later "foo-app",
					// we expect "bar-app" to be first in the order.
					assert.Equal(t, "bar-app", subAgent.Apps[0].Slug)
					assert.Equal(t, "foo-app", subAgent.Apps[1].Slug)
					assert.Equal(t, "baz-app", subAgent.Apps[2].Slug)

					// We do not expect the properties from the original "foo-app" to be
					// carried over.
					assert.Equal(t, false, subAgent.Apps[1].Hidden)
					assert.Equal(t, int32(2), subAgent.Apps[1].Order)
				},
			},
			{
				name: "Name",
				customization: agentcontainers.CoderCustomization{
					Name: "this-name",
				},
				mergedCustomizations: []agentcontainers.CoderCustomization{
					{
						Name: "not-this-name",
					},
					{
						Name: "or-this-name",
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.Equal(t, "this-name", subAgent.Name)
				},
			},
			{
				name: "NameIsOnlyUsedFromRoot",
				mergedCustomizations: []agentcontainers.CoderCustomization{
					{
						Name: "custom-name",
					},
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.NotEqual(t, "custom-name", subAgent.Name)
				},
			},
			{
				name: "EmptyNameIsIgnored",
				customization: agentcontainers.CoderCustomization{
					Name: "",
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.NotEmpty(t, subAgent.Name)
				},
			},
			{
				name: "InvalidNameIsIgnored",
				customization: agentcontainers.CoderCustomization{
					Name: "This--Is_An_Invalid--Name",
				},
				afterCreate: func(t *testing.T, subAgent agentcontainers.SubAgent) {
					require.NotEqual(t, "This--Is_An_Invalid--Name", subAgent.Name)
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
					fSAC   = &fakeSubAgentClient{
						logger:     logger.Named("fakeSubAgentClient"),
						createErrC: make(chan error, 1),
					}
					fDCCLI = &fakeDevcontainerCLI{
						readConfig: agentcontainers.DevcontainerConfig{
							Configuration: agentcontainers.DevcontainerConfiguration{
								Customizations: agentcontainers.DevcontainerCustomizations{
									Coder: tt.customization,
								},
							},
							MergedConfiguration: agentcontainers.DevcontainerMergedConfiguration{
								Customizations: agentcontainers.DevcontainerMergedCustomizations{
									Coder: tt.mergedCustomizations,
								},
							},
						},
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
					mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
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
				api.Start()
				defer api.Close()

				// Close before api.Close() defer to avoid deadlock after test.
				defer close(fSAC.createErrC)

				// Given: We allow agent creation and injection to succeed.
				testutil.RequireSend(ctx, t, fSAC.createErrC, nil)

				// Wait until the ticker has been registered.
				tickerTrap.MustWait(ctx).MustRelease(ctx)
				tickerTrap.Close()

				// Then: We expected it to succeed
				require.Len(t, fSAC.created, 1)

				if tt.afterCreate != nil {
					tt.afterCreate(t, fSAC.created[0])
				}
			})
		}
	})

	t.Run("CreateReadsConfigTwice", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		var (
			ctx    = testutil.Context(t, testutil.WaitMedium)
			logger = testutil.Logger(t)
			mClock = quartz.NewMock(t)
			mCCLI  = acmock.NewMockContainerCLI(gomock.NewController(t))
			fSAC   = &fakeSubAgentClient{
				logger:     logger.Named("fakeSubAgentClient"),
				createErrC: make(chan error, 1),
			}
			fDCCLI = &fakeDevcontainerCLI{
				readConfig: agentcontainers.DevcontainerConfig{
					Configuration: agentcontainers.DevcontainerConfiguration{
						Customizations: agentcontainers.DevcontainerCustomizations{
							Coder: agentcontainers.CoderCustomization{
								// We want to specify a custom name for this agent.
								Name: "custom-name",
							},
						},
					},
				},
				readConfigErrC: make(chan func(envs []string) error, 2),
			}

			testContainer = codersdk.WorkspaceAgentContainer{
				ID:           "test-container-id",
				FriendlyName: "test-container",
				Image:        "test-image",
				Running:      true,
				CreatedAt:    time.Now(),
				Labels: map[string]string{
					agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/coder",
					agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/coder/.devcontainer/devcontainer.json",
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
			mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
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
		api.Start()
		defer api.Close()

		// Close before api.Close() defer to avoid deadlock after test.
		defer close(fSAC.createErrC)
		defer close(fDCCLI.readConfigErrC)

		// Given: We allow agent creation and injection to succeed.
		testutil.RequireSend(ctx, t, fSAC.createErrC, nil)
		testutil.RequireSend(ctx, t, fDCCLI.readConfigErrC, func(env []string) error {
			// We expect the wrong workspace agent name passed in first.
			assert.Contains(t, env, "CODER_WORKSPACE_AGENT_NAME=coder")
			return nil
		})
		testutil.RequireSend(ctx, t, fDCCLI.readConfigErrC, func(env []string) error {
			// We then expect the agent name passed here to have been read from the config.
			assert.Contains(t, env, "CODER_WORKSPACE_AGENT_NAME=custom-name")
			assert.NotContains(t, env, "CODER_WORKSPACE_AGENT_NAME=coder")
			return nil
		})

		// Wait until the ticker has been registered.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Then: We expected it to succeed
		require.Len(t, fSAC.created, 1)
	})

	t.Run("ReadConfigWithFeatureOptions", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		var (
			ctx    = testutil.Context(t, testutil.WaitMedium)
			logger = testutil.Logger(t)
			mClock = quartz.NewMock(t)
			mCCLI  = acmock.NewMockContainerCLI(gomock.NewController(t))
			fSAC   = &fakeSubAgentClient{
				logger:     logger.Named("fakeSubAgentClient"),
				createErrC: make(chan error, 1),
			}
			fDCCLI = &fakeDevcontainerCLI{
				readConfig: agentcontainers.DevcontainerConfig{
					MergedConfiguration: agentcontainers.DevcontainerMergedConfiguration{
						Features: agentcontainers.DevcontainerFeatures{
							"./code-server": map[string]any{
								"port": 9090,
							},
							"ghcr.io/devcontainers/features/docker-in-docker:2": map[string]any{
								"moby": "false",
							},
						},
					},
					Workspace: agentcontainers.DevcontainerWorkspace{
						WorkspaceFolder: "/workspaces/coder",
					},
				},
				readConfigErrC: make(chan func(envs []string) error, 2),
			}

			testContainer = codersdk.WorkspaceAgentContainer{
				ID:           "test-container-id",
				FriendlyName: "test-container",
				Image:        "test-image",
				Running:      true,
				CreatedAt:    time.Now(),
				Labels: map[string]string{
					agentcontainers.DevcontainerLocalFolderLabel: "/workspaces/coder",
					agentcontainers.DevcontainerConfigFileLabel:  "/workspaces/coder/.devcontainer/devcontainer.json",
				},
			}
		)

		coderBin, err := os.Executable()
		require.NoError(t, err)

		// Mock the `List` function to always return our test container.
		mCCLI.EXPECT().List(gomock.Any()).Return(codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{testContainer},
		}, nil).AnyTimes()

		// Mock the steps used for injecting the coder agent.
		gomock.InOrder(
			mCCLI.EXPECT().DetectArchitecture(gomock.Any(), testContainer.ID).Return(runtime.GOARCH, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "mkdir", "-p", "/.coder-agent").Return(nil, nil),
			mCCLI.EXPECT().Copy(gomock.Any(), testContainer.ID, coderBin, "/.coder-agent/coder").Return(nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "chmod", "0755", "/.coder-agent", "/.coder-agent/coder").Return(nil, nil),
			mCCLI.EXPECT().ExecAs(gomock.Any(), testContainer.ID, "root", "/bin/sh", "-c", "chown $(id -u):$(id -g) /.coder-agent/coder").Return(nil, nil),
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
			agentcontainers.WithManifestInfo("test-user", "test-workspace", "test-parent-agent"),
		)
		api.Start()
		defer api.Close()

		// Close before api.Close() defer to avoid deadlock after test.
		defer close(fSAC.createErrC)
		defer close(fDCCLI.readConfigErrC)

		// Allow agent creation and injection to succeed.
		testutil.RequireSend(ctx, t, fSAC.createErrC, nil)

		testutil.RequireSend(ctx, t, fDCCLI.readConfigErrC, func(envs []string) error {
			assert.Contains(t, envs, "CODER_WORKSPACE_AGENT_NAME=coder")
			assert.Contains(t, envs, "CODER_WORKSPACE_NAME=test-workspace")
			assert.Contains(t, envs, "CODER_WORKSPACE_OWNER_NAME=test-user")
			assert.Contains(t, envs, "CODER_WORKSPACE_PARENT_AGENT_NAME=test-parent-agent")
			assert.Contains(t, envs, "CODER_URL=test-subagent-url")
			assert.Contains(t, envs, "CONTAINER_ID=test-container-id")
			// First call should not have feature envs.
			assert.NotContains(t, envs, "FEATURE_CODE_SERVER_OPTION_PORT=9090")
			assert.NotContains(t, envs, "FEATURE_DOCKER_IN_DOCKER_OPTION_MOBY=false")
			return nil
		})

		testutil.RequireSend(ctx, t, fDCCLI.readConfigErrC, func(envs []string) error {
			assert.Contains(t, envs, "CODER_WORKSPACE_AGENT_NAME=coder")
			assert.Contains(t, envs, "CODER_WORKSPACE_NAME=test-workspace")
			assert.Contains(t, envs, "CODER_WORKSPACE_OWNER_NAME=test-user")
			assert.Contains(t, envs, "CODER_WORKSPACE_PARENT_AGENT_NAME=test-parent-agent")
			assert.Contains(t, envs, "CODER_URL=test-subagent-url")
			assert.Contains(t, envs, "CONTAINER_ID=test-container-id")
			// Second call should have feature envs from the first config read.
			assert.Contains(t, envs, "FEATURE_CODE_SERVER_OPTION_PORT=9090")
			assert.Contains(t, envs, "FEATURE_DOCKER_IN_DOCKER_OPTION_MOBY=false")
			return nil
		})

		// Wait until the ticker has been registered.
		tickerTrap.MustWait(ctx).MustRelease(ctx)
		tickerTrap.Close()

		// Verify agent was created successfully
		require.Len(t, fSAC.created, 1)
	})

	t.Run("CommandEnv", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Create fake execer to track execution details.
		fakeExec := &fakeExecer{}

		// Custom CommandEnv that returns specific values.
		testShell := "/bin/custom-shell"
		testDir := t.TempDir()
		testEnv := []string{"CUSTOM_VAR=test_value", "PATH=/custom/path"}

		commandEnv := func(ei usershell.EnvInfoer, addEnv []string) (shell, dir string, env []string, err error) {
			return testShell, testDir, testEnv, nil
		}

		mClock := quartz.NewMock(t) // Stop time.

		// Create API with CommandEnv.
		api := agentcontainers.NewAPI(logger,
			agentcontainers.WithClock(mClock),
			agentcontainers.WithExecer(fakeExec),
			agentcontainers.WithCommandEnv(commandEnv),
		)
		api.Start()
		defer api.Close()

		// Call RefreshContainers directly to trigger CommandEnv usage.
		_ = api.RefreshContainers(ctx) // Ignore error since docker commands will fail.

		// Verify commands were executed through the custom shell and environment.
		require.NotEmpty(t, fakeExec.commands, "commands should be executed")

		// Want: /bin/custom-shell -c '"docker" "ps" "--all" "--quiet" "--no-trunc"'
		require.Equal(t, testShell, fakeExec.commands[0][0], "custom shell should be used")
		if runtime.GOOS == "windows" {
			require.Equal(t, "/c", fakeExec.commands[0][1], "shell should be called with /c on Windows")
		} else {
			require.Equal(t, "-c", fakeExec.commands[0][1], "shell should be called with -c")
		}
		require.Len(t, fakeExec.commands[0], 3, "command should have 3 arguments")
		require.GreaterOrEqual(t, strings.Count(fakeExec.commands[0][2], " "), 2, "command/script should have multiple arguments")
		require.True(t, strings.HasPrefix(fakeExec.commands[0][2], `"docker" "ps"`), "command should start with \"docker\" \"ps\"")

		// Verify the environment was set on the command.
		lastCmd := fakeExec.getLastCommand()
		require.NotNil(t, lastCmd, "command should be created")
		require.Equal(t, testDir, lastCmd.Dir, "custom directory should be used")
		require.Equal(t, testEnv, lastCmd.Env, "custom environment should be used")
	})

	t.Run("IgnoreCustomization", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Dev Container tests are not supported on Windows (this test uses mocks but fails due to Windows paths)")
		}

		ctx := testutil.Context(t, testutil.WaitShort)

		startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		configPath := "/workspace/project/.devcontainer/devcontainer.json"

		container := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Running:      true,
			CreatedAt:    startTime.Add(-1 * time.Hour),
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/workspace/project",
				agentcontainers.DevcontainerConfigFileLabel:  configPath,
			},
		}

		fLister := &fakeContainerCLI{
			containers: codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{container},
			},
			arch: runtime.GOARCH,
		}

		// Start with ignore=true
		fDCCLI := &fakeDevcontainerCLI{
			execErrC: make(chan func(string, ...string) error, 1),
			readConfig: agentcontainers.DevcontainerConfig{
				Configuration: agentcontainers.DevcontainerConfiguration{
					Customizations: agentcontainers.DevcontainerCustomizations{
						Coder: agentcontainers.CoderCustomization{Ignore: true},
					},
				},
				Workspace: agentcontainers.DevcontainerWorkspace{WorkspaceFolder: "/workspace/project"},
			},
		}

		fakeSAC := &fakeSubAgentClient{
			logger:     slogtest.Make(t, nil).Named("fakeSubAgentClient"),
			agents:     make(map[uuid.UUID]agentcontainers.SubAgent),
			createErrC: make(chan error, 1),
			deleteErrC: make(chan error, 1),
		}

		mClock := quartz.NewMock(t)
		mClock.Set(startTime)
		fWatcher := newFakeWatcher(t)

		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		api := agentcontainers.NewAPI(
			logger,
			agentcontainers.WithDevcontainerCLI(fDCCLI),
			agentcontainers.WithContainerCLI(fLister),
			agentcontainers.WithSubAgentClient(fakeSAC),
			agentcontainers.WithWatcher(fWatcher),
			agentcontainers.WithClock(mClock),
		)
		api.Start()
		defer func() {
			close(fakeSAC.createErrC)
			close(fakeSAC.deleteErrC)
			api.Close()
		}()

		err := api.RefreshContainers(ctx)
		require.NoError(t, err, "RefreshContainers should not error")

		r := chi.NewRouter()
		r.Mount("/", api.Routes())

		t.Log("Phase 1: Test ignore=true filters out devcontainer")
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var response codersdk.WorkspaceAgentListContainersResponse
		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)

		assert.Empty(t, response.Devcontainers, "ignored devcontainer should not be in response when ignore=true")
		assert.Len(t, response.Containers, 1, "regular container should still be listed")

		t.Log("Phase 2: Change to ignore=false")
		fDCCLI.readConfig.Configuration.Customizations.Coder.Ignore = false
		var (
			exitSubAgent     = make(chan struct{})
			subAgentExited   = make(chan struct{})
			exitSubAgentOnce sync.Once
		)
		defer func() {
			exitSubAgentOnce.Do(func() {
				close(exitSubAgent)
			})
		}()
		execSubAgent := func(cmd string, args ...string) error {
			if len(args) != 1 || args[0] != "agent" {
				t.Log("execSubAgent called with unexpected arguments", cmd, args)
				return nil
			}
			defer close(subAgentExited)
			select {
			case <-exitSubAgent:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		}
		testutil.RequireSend(ctx, t, fDCCLI.execErrC, execSubAgent)
		testutil.RequireSend(ctx, t, fakeSAC.createErrC, nil)

		fWatcher.sendEventWaitNextCalled(ctx, fsnotify.Event{
			Name: configPath,
			Op:   fsnotify.Write,
		})

		err = api.RefreshContainers(ctx)
		require.NoError(t, err)

		t.Log("Phase 2: Cont, waiting for sub agent to exit")
		exitSubAgentOnce.Do(func() {
			close(exitSubAgent)
		})
		select {
		case <-subAgentExited:
		case <-ctx.Done():
			t.Fatal("timeout waiting for sub agent to exit")
		}

		req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)

		assert.Len(t, response.Devcontainers, 1, "devcontainer should be in response when ignore=false")
		assert.Len(t, response.Containers, 1, "regular container should still be listed")
		assert.Equal(t, "/workspace/project", response.Devcontainers[0].WorkspaceFolder)
		require.Len(t, fakeSAC.created, 1, "sub agent should be created when ignore=false")
		createdAgentID := fakeSAC.created[0].ID

		t.Log("Phase 3: Change back to ignore=true and test sub agent deletion")
		fDCCLI.readConfig.Configuration.Customizations.Coder.Ignore = true
		testutil.RequireSend(ctx, t, fakeSAC.deleteErrC, nil)

		fWatcher.sendEventWaitNextCalled(ctx, fsnotify.Event{
			Name: configPath,
			Op:   fsnotify.Write,
		})

		err = api.RefreshContainers(ctx)
		require.NoError(t, err)

		req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		err = json.NewDecoder(rec.Body).Decode(&response)
		require.NoError(t, err)

		assert.Empty(t, response.Devcontainers, "devcontainer should be filtered out when ignore=true again")
		assert.Len(t, response.Containers, 1, "regular container should still be listed")
		require.Len(t, fakeSAC.deleted, 1, "sub agent should be deleted when ignore=true")
		assert.Equal(t, createdAgentID, fakeSAC.deleted[0], "the same sub agent that was created should be deleted")
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

// TestSubAgentCreationWithNameRetry tests the retry logic when unique constraint violations occur
func TestSubAgentCreationWithNameRetry(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Dev Container tests are not supported on Windows")
	}

	tests := []struct {
		name             string
		workspaceFolders []string
		expectedNames    []string
		takenNames       []string
	}{
		{
			name: "SingleCollision",
			workspaceFolders: []string{
				"/home/coder/foo/project",
				"/home/coder/bar/project",
			},
			expectedNames: []string{
				"project",
				"bar-project",
			},
		},
		{
			name: "MultipleCollisions",
			workspaceFolders: []string{
				"/home/coder/foo/x/project",
				"/home/coder/bar/x/project",
				"/home/coder/baz/x/project",
			},
			expectedNames: []string{
				"project",
				"x-project",
				"baz-x-project",
			},
		},
		{
			name:       "NameAlreadyTaken",
			takenNames: []string{"project", "x-project"},
			workspaceFolders: []string{
				"/home/coder/foo/x/project",
			},
			expectedNames: []string{
				"foo-x-project",
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
				fSAC   = &fakeSubAgentClient{logger: logger, agents: make(map[uuid.UUID]agentcontainers.SubAgent)}
				ccli   = &fakeContainerCLI{arch: runtime.GOARCH}
			)

			for _, name := range tt.takenNames {
				fSAC.agents[uuid.New()] = agentcontainers.SubAgent{Name: name}
			}

			mClock.Set(time.Now()).MustWait(ctx)
			tickerTrap := mClock.Trap().TickerFunc("updaterLoop")

			api := agentcontainers.NewAPI(logger,
				agentcontainers.WithClock(mClock),
				agentcontainers.WithContainerCLI(ccli),
				agentcontainers.WithDevcontainerCLI(&fakeDevcontainerCLI{}),
				agentcontainers.WithSubAgentClient(fSAC),
				agentcontainers.WithWatcher(watcher.NewNoop()),
			)
			api.Start()
			defer api.Close()

			tickerTrap.MustWait(ctx).MustRelease(ctx)
			tickerTrap.Close()

			for i, workspaceFolder := range tt.workspaceFolders {
				ccli.containers.Containers = append(ccli.containers.Containers, newFakeContainer(
					fmt.Sprintf("container%d", i+1),
					fmt.Sprintf("/.devcontainer/devcontainer%d.json", i+1),
					workspaceFolder,
				))

				err := api.RefreshContainers(ctx)
				require.NoError(t, err)
			}

			// Verify that both agents were created with expected names
			require.Len(t, fSAC.created, len(tt.workspaceFolders))

			actualNames := make([]string, len(fSAC.created))
			for i, agent := range fSAC.created {
				actualNames[i] = agent.Name
			}

			slices.Sort(tt.expectedNames)
			slices.Sort(actualNames)

			assert.Equal(t, tt.expectedNames, actualNames)
		})
	}
}

func newFakeContainer(id, configPath, workspaceFolder string) codersdk.WorkspaceAgentContainer {
	return codersdk.WorkspaceAgentContainer{
		ID:           id,
		FriendlyName: "test-friendly",
		Image:        "test-image:latest",
		Labels: map[string]string{
			agentcontainers.DevcontainerLocalFolderLabel: workspaceFolder,
			agentcontainers.DevcontainerConfigFileLabel:  configPath,
		},
		Running: true,
	}
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

func TestWithDevcontainersNameGeneration(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Dev Container tests are not supported on Windows")
	}

	devcontainers := []codersdk.WorkspaceAgentDevcontainer{
		{
			ID:              uuid.New(),
			Name:            "original-name",
			WorkspaceFolder: "/home/coder/foo/project",
			ConfigPath:      "/home/coder/foo/project/.devcontainer/devcontainer.json",
		},
		{
			ID:              uuid.New(),
			Name:            "another-name",
			WorkspaceFolder: "/home/coder/bar/project",
			ConfigPath:      "/home/coder/bar/project/.devcontainer/devcontainer.json",
		},
	}

	scripts := []codersdk.WorkspaceAgentScript{
		{ID: devcontainers[0].ID, LogSourceID: uuid.New()},
		{ID: devcontainers[1].ID, LogSourceID: uuid.New()},
	}

	logger := testutil.Logger(t)

	// This should trigger the WithDevcontainers code path where names are generated
	api := agentcontainers.NewAPI(logger,
		agentcontainers.WithDevcontainers(devcontainers, scripts),
		agentcontainers.WithContainerCLI(&fakeContainerCLI{
			containers: codersdk.WorkspaceAgentListContainersResponse{
				Containers: []codersdk.WorkspaceAgentContainer{
					fakeContainer(t, func(c *codersdk.WorkspaceAgentContainer) {
						c.ID = "some-container-id-1"
						c.FriendlyName = "container-name-1"
						c.Labels[agentcontainers.DevcontainerLocalFolderLabel] = "/home/coder/baz/project"
						c.Labels[agentcontainers.DevcontainerConfigFileLabel] = "/home/coder/baz/project/.devcontainer/devcontainer.json"
					}),
				},
			},
		}),
		agentcontainers.WithDevcontainerCLI(&fakeDevcontainerCLI{}),
		agentcontainers.WithSubAgentClient(&fakeSubAgentClient{}),
		agentcontainers.WithWatcher(watcher.NewNoop()),
	)
	defer api.Close()
	api.Start()

	r := chi.NewRouter()
	r.Mount("/", api.Routes())

	ctx := context.Background()

	err := api.RefreshContainers(ctx)
	require.NoError(t, err, "RefreshContainers should not error")

	// Initial request returns the initial data.
	req := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(ctx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response codersdk.WorkspaceAgentListContainersResponse
	err = json.NewDecoder(rec.Body).Decode(&response)
	require.NoError(t, err)

	// Verify the devcontainers have the expected names.
	require.Len(t, response.Devcontainers, 3, "should have two devcontainers")
	assert.NotEqual(t, "original-name", response.Devcontainers[2].Name, "first devcontainer should not keep original name")
	assert.Equal(t, "project", response.Devcontainers[2].Name, "first devcontainer should use the project folder name")
	assert.NotEqual(t, "another-name", response.Devcontainers[0].Name, "second devcontainer should not keep original name")
	assert.Equal(t, "bar-project", response.Devcontainers[0].Name, "second devcontainer should has a collision and uses the folder name with a prefix")
	assert.Equal(t, "baz-project", response.Devcontainers[1].Name, "third devcontainer should use the folder name with a prefix since it collides with the first two")
}
