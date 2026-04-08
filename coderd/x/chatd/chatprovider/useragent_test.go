package chatprovider_test

import (
	"runtime"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
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
	ctx := testutil.Context(t, testutil.WaitShort)

	expectedUA := chatprovider.UserAgent()
	called := make(chan struct{})
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		assert.Equal(t, expectedUA, req.Header.Get("User-Agent"))
		close(called)
		return chattest.OpenAINonStreamingResponse("hello")
	})

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"openai": "test-key"},
		BaseURLByProvider: map[string]string{"openai": serverURL},
	}

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4", keys, expectedUA, nil)
	require.NoError(t, err)

	// Make a real call so Fantasy sends an HTTP request to the
	// fake server, which asserts the User-Agent header.
	_, err = model.Generate(ctx, fantasy.Call{
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
	_ = testutil.TryReceive(ctx, t, called)
}
