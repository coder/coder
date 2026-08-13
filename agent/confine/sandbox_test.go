package confine_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSandboxDeclarationFromEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         map[string]string
		want        confine.SandboxDeclaration
		errorString string
	}{
		{
			name: "defaults",
			env: map[string]string{
				confine.EnvAISandboxCreateScript: "create-sandbox",
			},
			want: confine.SandboxDeclaration{
				CreateScript:      "create-sandbox",
				Name:              "sandbox",
				EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
				ProxyAddress:      "127.0.0.1:0",
			},
		},
		{
			name: "explicit",
			env: map[string]string{
				confine.EnvAISandboxCreateScript:      "create-sandbox",
				confine.EnvAISandboxDestroyScript:     "destroy-sandbox",
				confine.EnvAISandboxName:              "sandbox_1",
				confine.EnvAISandboxEgressEnforcement: "forced",
				confine.EnvAISandboxProxyAddress:      "192.0.2.1:0",
			},
			want: confine.SandboxDeclaration{
				CreateScript:      "create-sandbox",
				DestroyScript:     "destroy-sandbox",
				Name:              "sandbox_1",
				EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
				ProxyAddress:      "192.0.2.1:0",
			},
		},
		{
			name: "invalid enforcement",
			env: map[string]string{
				confine.EnvAISandboxCreateScript:      "create-sandbox",
				confine.EnvAISandboxEgressEnforcement: "sometimes",
			},
			errorString: "invalid AI sandbox egress enforcement",
		},
		{
			name: "invalid name",
			env: map[string]string{
				confine.EnvAISandboxCreateScript: "create-sandbox",
				confine.EnvAISandboxName:         "invalid.name",
			},
			errorString: "invalid AI sandbox name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			declaration, err := confine.SandboxDeclarationFromEnv(func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			})
			if test.errorString != "" {
				require.ErrorContains(t, err, test.errorString)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, declaration)
		})
	}
}

//nolint:paralleltest // The test verifies platform precedence over process environment variables.
func TestSandboxControllerHappyPath(t *testing.T) {
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, "create.env")
	t.Setenv(confine.EnvAIAgentToken, "preexisting-agent-token")

	state := newSandboxServerState()
	declaration := confine.SandboxDeclaration{
		CreateScript:      fmt.Sprintf("env > %q", envFile),
		Name:              "sandbox",
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
		ProxyAddress:      "127.0.0.1:0",
	}
	controller, accessURL := newSandboxController(t, state, declaration, tempDir)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	readyCtx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, controller.WaitForProxy(readyCtx))
	workspaceScriptEnv := environmentMap(strings.Join(controller.ScriptExtraEnv(), "\n") + "\n")
	require.Equal(t, state.response.ID.String(), workspaceScriptEnv[confine.EnvSandboxID])

	var createEnv string
	eventuallyCtx := testutil.Context(t, testutil.WaitLong)
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(context.Context) bool {
		contents, err := os.ReadFile(envFile)
		if err != nil {
			return false
		}
		createEnv = string(contents)
		return true
	}, testutil.IntervalFast))

	environment := environmentMap(createEnv)
	require.Equal(t, accessURL.String(), environment[confine.EnvAIAgentURL])
	require.Equal(t, state.response.AgentToken, environment[confine.EnvAIAgentToken])
	require.Equal(t, state.response.SessionToken, environment[confine.EnvAISessionToken])
	require.Equal(t, state.response.ID.String(), environment[confine.EnvSandboxID])
	proxyAddress := environment[confine.EnvEgressProxy]
	require.NotEmpty(t, proxyAddress)
	require.Equal(t, proxyAddress, workspaceScriptEnv[confine.EnvEgressProxy])
	require.Equal(t, 1, countEnvironmentKey(createEnv, confine.EnvAIAgentToken))

	// The exact control-plane host is implicitly allowed by policy and may
	// resolve to loopback or another private address in development and on-prem
	// deployments. The controller passes that one hostname through destination
	// validation without weakening private-address denial for any other host.
	proxyURL, err := url.Parse("http://" + proxyAddress)
	require.NoError(t, err)
	proxyClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	controlRequest, err := http.NewRequestWithContext(readyCtx, http.MethodGet, accessURL.String(), nil)
	require.NoError(t, err)
	controlResponse, err := proxyClient.Do(controlRequest)
	require.NoError(t, err)
	require.NoError(t, controlResponse.Body.Close())

	connection, err := net.Dial("tcp", proxyAddress)
	require.NoError(t, err)
	_, err = fmt.Fprintf(connection, "CONNECT denied.example:443 HTTP/1.1\r\nHost: denied.example:443\r\n\r\n")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.NoError(t, connection.Close())

	cancel()
	require.NoError(t, <-errCh)

	state.mu.Lock()
	defer state.mu.Unlock()
	require.Equal(t, agentsdk.CreateAISandboxRequest{
		Name:              "sandbox",
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
	}, state.createRequests[0])
	require.GreaterOrEqual(t, len(state.sessions), 2)
	require.Equal(t, state.response.ChildAgentID, state.sessions[0].ChildAgentID)
	require.Nil(t, state.sessions[0].EndedAt)
	require.NotNil(t, state.sessions[len(state.sessions)-1].EndedAt)
	require.Len(t, state.eventBatches, 1)
	require.Len(t, state.eventBatches[0].Events, 2)
	require.Equal(t, accessURL.Hostname(), state.eventBatches[0].Events[0].Host)
	require.Equal(t, agentsdk.AISandboxNetworkEventActionAllowed, state.eventBatches[0].Events[0].Action)
	require.Equal(t, "denied.example", state.eventBatches[0].Events[1].Host)
	require.Equal(t, agentsdk.AISandboxNetworkEventActionDenied, state.eventBatches[0].Events[1].Action)
}

func TestSandboxControllerDeletesStaleSandbox(t *testing.T) {
	t.Parallel()

	state := newSandboxServerState()
	staleID := uuid.New()
	state.sandboxes = []agentsdk.AISandbox{{
		ID:           staleID,
		ChildAgentID: uuid.New(),
		Name:         "stale",
	}}
	declaration := confine.SandboxDeclaration{
		CreateScript:      ":",
		Name:              "current",
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
		ProxyAddress:      "127.0.0.1:0",
	}
	controller, _ := newSandboxController(t, state, declaration, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	eventuallyCtx := testutil.Context(t, testutil.WaitLong)
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(context.Context) bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		return len(state.sessions) > 0
	}, testutil.IntervalFast))
	cancel()
	require.NoError(t, <-errCh)

	state.mu.Lock()
	defer state.mu.Unlock()
	require.Equal(t, []uuid.UUID{staleID}, state.deleted)
}

//nolint:paralleltest // The test verifies that the destroy environment strips a process variable.
func TestSandboxControllerDestroyAndSessionClose(t *testing.T) {
	tempDir := t.TempDir()
	markerFile := filepath.Join(tempDir, "destroyed")
	t.Setenv(confine.EnvAISessionToken, "preexisting-session-token")

	state := newSandboxServerState()
	declaration := confine.SandboxDeclaration{
		CreateScript:      ":",
		DestroyScript:     fmt.Sprintf(`[ -z "${%s+x}" ] && printf destroyed > %q`, confine.EnvAISessionToken, markerFile),
		Name:              "sandbox",
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		ProxyAddress:      "127.0.0.1:0",
	}
	controller, _ := newSandboxController(t, state, declaration, tempDir)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- controller.Run(ctx)
	}()

	eventuallyCtx := testutil.Context(t, testutil.WaitLong)
	require.True(t, testutil.Eventually(eventuallyCtx, t, func(context.Context) bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		return len(state.sessions) > 0
	}, testutil.IntervalFast))
	cancel()
	require.NoError(t, <-errCh)

	contents, err := os.ReadFile(markerFile)
	require.NoError(t, err)
	require.Equal(t, "destroyed", string(contents))

	state.mu.Lock()
	defer state.mu.Unlock()
	require.NotNil(t, state.sessions[len(state.sessions)-1].EndedAt)
	require.Equal(t, state.response.ChildAgentID, state.sessions[len(state.sessions)-1].ChildAgentID)
}

func newSandboxController(
	t *testing.T,
	state *sandboxServerState,
	declaration confine.SandboxDeclaration,
	logDir string,
) (*confine.SandboxController, *url.URL) {
	t.Helper()
	server := httptest.NewServer(state)
	t.Cleanup(server.Close)
	accessURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := agentsdk.New(accessURL, agentsdk.WithFixedToken("parent-agent-token"))
	controller, err := confine.NewSandboxController(confine.SandboxControllerOptions{
		Declaration: declaration,
		Client:      client,
		Logger:      slog.Make(),
		LogDir:      logDir,
		AccessURL:   accessURL,
	})
	require.NoError(t, err)
	return controller, accessURL
}

func environmentMap(contents string) map[string]string {
	environment := make(map[string]string)
	for line := range strings.Lines(contents) {
		key, value, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "=")
		if ok {
			environment[key] = value
		}
	}
	return environment
}

func countEnvironmentKey(contents, key string) int {
	count := 0
	prefix := key + "="
	for line := range strings.Lines(contents) {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

type sandboxServerState struct {
	mu sync.Mutex

	response       agentsdk.CreateAISandboxResponse
	sandboxes      []agentsdk.AISandbox
	createRequests []agentsdk.CreateAISandboxRequest
	deleted        []uuid.UUID
	sessions       []agentsdk.PostAISandboxSessionRequest
	eventBatches   []agentsdk.PatchAISandboxNetworkEventsRequest
	logs           []agentsdk.PatchLogs
}

func newSandboxServerState() *sandboxServerState {
	return &sandboxServerState{
		response: agentsdk.CreateAISandboxResponse{
			ID:           uuid.New(),
			ChildAgentID: uuid.New(),
			AIAgentID:    uuid.New(),
			AgentToken:   "child-agent-token",
			SessionToken: "sandbox-session-token",
		},
	}
}

func (s *sandboxServerState) ServeHTTP(rw http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/v2/workspaceagents/me/ai-sandboxes":
		s.mu.Lock()
		sandboxes := append([]agentsdk.AISandbox(nil), s.sandboxes...)
		s.mu.Unlock()
		writeJSON(rw, sandboxes)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v2/workspaceagents/me/ai-sandboxes":
		var createRequest agentsdk.CreateAISandboxRequest
		if json.NewDecoder(request.Body).Decode(&createRequest) != nil {
			http.Error(rw, "invalid request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.createRequests = append(s.createRequests, createRequest)
		response := s.response
		s.mu.Unlock()
		writeJSON(rw, response)
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/v2/workspaceagents/me/ai-sandboxes/"):
		id, err := uuid.Parse(strings.TrimPrefix(request.URL.Path, "/api/v2/workspaceagents/me/ai-sandboxes/"))
		if err != nil {
			http.Error(rw, "invalid sandbox id", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.deleted = append(s.deleted, id)
		s.mu.Unlock()
		writeJSON(rw, map[string]string{})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v2/workspaceagents/me/ai-egress-policy":
		writeJSON(rw, codersdk.AIEgressPolicy{Revision: 7})
	case request.Method == http.MethodGet && request.URL.Path == "/api/v2/workspaceagents/me/ai-egress-policy/watch":
		rw.Header().Set("Content-Type", "text/event-stream")
		rw.WriteHeader(http.StatusOK)
		if flusher, ok := rw.(http.Flusher); ok {
			flusher.Flush()
		}
		<-request.Context().Done()
	case request.Method == http.MethodPost && request.URL.Path == "/api/v2/workspaceagents/me/ai-sandbox-sessions":
		var session agentsdk.PostAISandboxSessionRequest
		if json.NewDecoder(request.Body).Decode(&session) != nil {
			http.Error(rw, "invalid request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.sessions = append(s.sessions, session)
		s.mu.Unlock()
		writeJSON(rw, map[string]string{})
	case request.Method == http.MethodPatch && request.URL.Path == "/api/v2/workspaceagents/me/ai-sandbox-network-events":
		var eventBatch agentsdk.PatchAISandboxNetworkEventsRequest
		if json.NewDecoder(request.Body).Decode(&eventBatch) != nil {
			http.Error(rw, "invalid request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.eventBatches = append(s.eventBatches, eventBatch)
		s.mu.Unlock()
		writeJSON(rw, map[string]string{})
	case request.Method == http.MethodPatch && request.URL.Path == "/api/v2/workspaceagents/me/logs":
		var logs agentsdk.PatchLogs
		if json.NewDecoder(request.Body).Decode(&logs) != nil {
			http.Error(rw, "invalid request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.logs = append(s.logs, logs)
		s.mu.Unlock()
		writeJSON(rw, map[string]string{})
	default:
		http.NotFound(rw, request)
	}
}

func writeJSON(rw http.ResponseWriter, value any) {
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(value)
}

// TestSandboxDeclarationProxyOnly covers the declarative surface: a template
// that declares an ai_bound coder_agent and builds the sandbox from an
// ordinary coder_script gives the agent no create script, but the agent must
// still start the egress proxy so scripts can route through it.
func TestSandboxDeclarationProxyOnly(t *testing.T) {
	t.Parallel()

	t.Run("EnablesWithoutCreateScript", func(t *testing.T) {
		t.Parallel()

		declaration, err := confine.SandboxDeclarationFromEnv(mapLookup(map[string]string{
			confine.EnvAIEgressProxy:              "true",
			confine.EnvAISandboxProxyAddress:      "0.0.0.0:0",
			confine.EnvAISandboxEgressEnforcement: "forced",
		}))
		require.NoError(t, err)
		require.Empty(t, declaration.CreateScript,
			"proxy-only mode must not invent a create script")
		require.Equal(t, "0.0.0.0:0", declaration.ProxyAddress)
	})

	t.Run("RequiresOneOfTheTwoSurfaces", func(t *testing.T) {
		t.Parallel()

		_, err := confine.SandboxDeclarationFromEnv(mapLookup(map[string]string{
			confine.EnvAISandboxName: "demo",
		}))
		require.Error(t, err, "neither surface declared must be an error, not a silent no-op")
	})

	t.Run("CreateScriptStillWins", func(t *testing.T) {
		t.Parallel()

		declaration, err := confine.SandboxDeclarationFromEnv(mapLookup(map[string]string{
			confine.EnvAISandboxCreateScript: "/opt/up.sh",
			confine.EnvAIEgressProxy:         "true",
		}))
		require.NoError(t, err)
		require.Equal(t, "/opt/up.sh", declaration.CreateScript,
			"a declared create script keeps the platform-managed sandbox path")
	})
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
