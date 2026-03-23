package chatprovider_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestUserAgent(t *testing.T) {
	t.Parallel()
	ua := chatprovider.UserAgent()

	// Must start with "coder-agents/" so LLM providers can
	// identify traffic from Coder.
	require.True(t, strings.HasPrefix(ua, "coder-agents/"),
		"User-Agent should start with 'coder-agents/', got %q", ua)

	// Must contain the build version.
	assert.Contains(t, ua, buildinfo.Version())

	// Must contain OS/arch.
	assert.Contains(t, ua, runtime.GOOS+"/"+runtime.GOARCH)
}

func TestModelFromConfig_UserAgent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var capturedUA string

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		mu.Lock()
		capturedUA = req.Header.Get("User-Agent")
		mu.Unlock()
		return chattest.OpenAINonStreamingResponse("hello")
	})

	expectedUA := chatprovider.UserAgent()
	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"openai": "test-key"},
		BaseURLByProvider: map[string]string{"openai": serverURL},
	}

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, expectedUA)
	require.NoError(t, err)

	// Make a real call so Fantasy sends an HTTP request to the
	// fake server, which captures the User-Agent header.
	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hello"},
				},
			},
		},
	})
	require.NoError(t, err)

	mu.Lock()
	got := capturedUA
	mu.Unlock()

	require.NotEmpty(t, got, "User-Agent header was not sent")
	require.Equal(t, expectedUA, got,
		"User-Agent header should match chatprovider.UserAgent()")
}
