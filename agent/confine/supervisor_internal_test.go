package confine

import (
	"context"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChildEnvironment(t *testing.T) {
	t.Parallel()

	t.Run("proxy only", func(t *testing.T) {
		t.Parallel()
		env := childEnvironment([]string{"OTHER=value", "CODER_AGENT_CONFINE=proxy"}, "http://127.0.0.1:1234", codersdk.AISandboxEgressEnforcementAdvisory)
		require.Equal(t, map[string]string{
			"OTHER":                "value",
			EnvAgentEgressProxyURL: "http://127.0.0.1:1234",
		}, envMap(env))
	})

	t.Run("forced", func(t *testing.T) {
		t.Parallel()
		env := childEnvironment([]string{
			"OTHER=value",
			"CODER_AGENT_CONFINE=netns",
			"HTTP_PROXY=http://old",
			"NO_PROXY=old",
		}, "http://100.115.92.1:1234", codersdk.AISandboxEgressEnforcementForced)
		require.Equal(t, map[string]string{
			"OTHER":                "value",
			EnvAgentEgressProxyURL: "http://100.115.92.1:1234",
			"HTTP_PROXY":           "http://100.115.92.1:1234",
			"HTTPS_PROXY":          "http://100.115.92.1:1234",
			"http_proxy":           "http://100.115.92.1:1234",
			"https_proxy":          "http://100.115.92.1:1234",
			"NO_PROXY":             "localhost,127.0.0.1,::1",
		}, envMap(env))
	})
}

// Namespace mode is a structural claim, so an environment that cannot
// provide it must fail rather than quietly serve the weaker advisory
// boundary that an earlier revision fell back to.
//
//nolint:paralleltest // t.Setenv temporarily removes PATH to simulate missing tooling.
func TestSupervisorNetNSFailsClosed(t *testing.T) {
	t.Setenv("PATH", "")
	client := &fakeAgentClient{}
	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	supervisor, err := NewSupervisor(SupervisorOptions{
		Mode:      ModeNetNS,
		Client:    client,
		Logger:    slog.Make(),
		AccessURL: accessURL,
		ExecArgs:  []string{"/bin/true"},
	})
	require.NoError(t, err)

	exitCode, err := supervisor.Run(testutil.Context(t, testutil.WaitShort))
	require.Error(t, err)
	require.Equal(t, 1, exitCode)
	require.Contains(t, err.Error(), "network confinement unavailable")

	client.mu.Lock()
	defer client.mu.Unlock()
	for _, patch := range client.logs {
		for _, entry := range patch.Logs {
			require.NotContains(t, entry.Output, "using proxy-only advisory mode",
				"forced confinement must not downgrade to advisory")
		}
	}
}

func TestEventBatcherDropsOldest(t *testing.T) {
	t.Parallel()

	client := &fakeAgentClient{}
	batcher := newEventBatcher(client, slog.Make(), uuid.New(), 2)
	batcher.Add(NetworkEvent{Host: "one.example.com"})
	batcher.Add(NetworkEvent{Host: "two.example.com"})
	batcher.Add(NetworkEvent{Host: "three.example.com"})
	batcher.Flush()

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Len(t, client.eventBatches, 1)
	events := client.eventBatches[0].Events
	require.Len(t, events, 2)
	require.Equal(t, "two.example.com", events[0].Host)
	require.Equal(t, "three.example.com", events[1].Host)
}

func envMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, value := range env {
		key, val, ok := strings.Cut(value, "=")
		if ok {
			result[key] = val
		}
	}
	return result
}

type fakeAgentClient struct {
	mu           sync.Mutex
	logs         []agentsdk.PatchLogs
	eventBatches []agentsdk.PatchAISandboxNetworkEventsRequest
}

func (*fakeAgentClient) AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error) {
	return codersdk.AIEgressPolicy{}, xerrors.New("not implemented")
}

func (*fakeAgentClient) WatchAIEgressPolicy(context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error) {
	return nil, nil, xerrors.New("not implemented")
}

func (*fakeAgentClient) PostAISandboxSession(context.Context, agentsdk.PostAISandboxSessionRequest) error {
	return nil
}

func (f *fakeAgentClient) PatchAISandboxNetworkEvents(_ context.Context, request agentsdk.PatchAISandboxNetworkEventsRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventBatches = append(f.eventBatches, request)
	return nil
}

func (f *fakeAgentClient) PatchLogs(_ context.Context, logs agentsdk.PatchLogs) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, logs)
	return nil
}
