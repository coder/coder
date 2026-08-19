package chatprovider_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/internal/googleopenai"
)

func TestModelFromConfig_GeminiOpenAICompatThoughtSignatures(t *testing.T) {
	t.Parallel()

	t.Run("Gemini endpoint receives current turn thought signature", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "https://generativelanguage.googleapis.com/v1beta/openai/", "gemini-3.5-flash")
		messages := body["messages"].([]any)

		require.Empty(t, thoughtSignature(t, messages[1], 0))
		require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, messages[4], 0))
		require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, messages[4], 1))
		require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, messages[6], 0))
	})

	t.Run("Coder AI Bridge Gemini route receives current turn thought signature", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "http://coder-aibridge/v1", "gemini-3.5-flash")
		messages := body["messages"].([]any)

		require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, messages[4], 0))
	})

	t.Run("Vercel OpenAI-compatible Gemini route is unchanged", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "https://gateway.vercel.ai/v1", "google/gemini-3.5-flash")
		messages := body["messages"].([]any)

		require.Empty(t, thoughtSignature(t, messages[4], 0))
	})
}

func generateOpenAICompatRequest(t *testing.T, baseURL string, modelID string, providerOptions ...fantasy.ProviderOptions) map[string]any {
	t.Helper()

	transport := &captureChatCompletionTransport{}
	model, err := chatprovider.ModelFromConfig(
		fantasyopenaicompat.Name,
		modelID,
		chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				fantasyopenaicompat.Name: "test-key",
			},
			BaseURLByProvider: map[string]string{
				fantasyopenaicompat.Name: baseURL,
			},
		},
		chatprovider.UserAgent(),
		nil,
		&http.Client{Transport: transport},
		nil,
	)
	require.NoError(t, err)

	call := fantasy.Call{
		Prompt: geminiOpenAICompatToolPrompt(),
	}
	if len(providerOptions) > 0 {
		call.ProviderOptions = providerOptions[0]
	}
	_, err = model.LanguageModel().Generate(t.Context(), call)
	require.NoError(t, err)
	require.NotNil(t, transport.body)
	return transport.body
}

type captureChatCompletionTransport struct {
	body map[string]any
}

func (ct *captureChatCompletionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	if strings.HasSuffix(req.URL.Path, "/chat/completions") {
		ct.body = map[string]any{}
		if err := json.Unmarshal(body, &ct.body); err != nil {
			return nil, err
		}
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"gemini-3.5-flash",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`)),
	}, nil
}

func geminiOpenAICompatToolPrompt() []fantasy.Message {
	return []fantasy.Message{
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "previous turn"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{ToolCallID: "previous-call", ToolName: "previous_tool", Input: `{}`},
			},
		},
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "previous-call",
					Output:     fantasy.ToolResultOutputContentText{Text: `{}`},
				},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "current turn"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{ToolCallID: "current-call-a", ToolName: "first_tool", Input: `{}`},
				fantasy.ToolCallPart{ToolCallID: "current-call-b", ToolName: "parallel_tool", Input: `{}`},
			},
		},
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID: "current-call-a",
					Output:     fantasy.ToolResultOutputContentText{Text: `{}`},
				},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID: "current-call-c",
					ToolName:   "second_step_tool",
					Input:      `{}`,
				},
			},
		},
	}
}

func thoughtSignature(t *testing.T, rawMessage any, toolCallIndex int) string {
	t.Helper()
	message, ok := rawMessage.(map[string]any)
	require.True(t, ok)
	toolCalls, ok := message["tool_calls"].([]any)
	require.True(t, ok)
	require.Greater(t, len(toolCalls), toolCallIndex)
	toolCall, ok := toolCalls[toolCallIndex].(map[string]any)
	require.True(t, ok)
	extraContent, _ := toolCall["extra_content"].(map[string]any)
	google, _ := extraContent["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	return signature
}

func TestModelFromConfig_GeminiOpenAICompatThinkingConfig(t *testing.T) {
	t.Parallel()

	effort := fantasyopenai.ReasoningEffort("medium")
	effortOptions := fantasy.ProviderOptions{
		fantasyopenaicompat.Name: &fantasyopenaicompat.ProviderOptions{ReasoningEffort: &effort},
	}

	t.Run("Gemini endpoint swaps reasoning_effort for thinking_config", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "https://generativelanguage.googleapis.com/v1beta/openai/", "gemini-3-flash-preview", effortOptions)

		require.NotContains(t, body, "reasoning_effort")
		extraBody := body["extra_body"].(map[string]any)
		google := extraBody["google"].(map[string]any)
		require.Equal(t, map[string]any{
			"include_thoughts": true,
			"thinking_level":   "medium",
		}, google["thinking_config"])
	})

	t.Run("Coder AI Bridge Gemini route swaps reasoning_effort for thinking_config", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "http://coder-aibridge/v1", "gemini-2.5-flash", effortOptions)

		require.NotContains(t, body, "reasoning_effort")
		extraBody := body["extra_body"].(map[string]any)
		google := extraBody["google"].(map[string]any)
		require.Equal(t, map[string]any{
			"include_thoughts": true,
			"thinking_budget":  float64(8192),
		}, google["thinking_config"])
	})

	t.Run("Vercel OpenAI-compatible Gemini route is unchanged", func(t *testing.T) {
		t.Parallel()

		body := generateOpenAICompatRequest(t, "https://gateway.vercel.ai/v1", "google/gemini-3-flash-preview", effortOptions)

		require.Equal(t, "medium", body["reasoning_effort"])
		require.NotContains(t, body, "extra_body")
	})
}

func TestModelFromConfig_GeminiOpenAICompatThoughtStreaming(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"<thought>**Calculating**\n\nStep one.","extra_content":{"google":{"thought":true}}},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"More thinking.","extra_content":{"google":{"thought":true}}},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"</thought>The answer is "},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"391."},"index":0,"finish_reason":"stop"}],"object":"chat.completion.chunk"}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	transport := &sseChatCompletionTransport{stream: stream}
	model, err := chatprovider.ModelFromConfig(
		fantasyopenaicompat.Name,
		"gemini-3-flash-preview",
		chatprovider.ProviderAPIKeys{
			ByProvider: map[string]string{
				fantasyopenaicompat.Name: "test-key",
			},
			BaseURLByProvider: map[string]string{
				fantasyopenaicompat.Name: "https://generativelanguage.googleapis.com/v1beta/openai/",
			},
		},
		chatprovider.UserAgent(),
		nil,
		&http.Client{Transport: transport},
		nil,
	)
	require.NoError(t, err)

	parts, err := model.LanguageModel().Stream(t.Context(), fantasy.Call{
		Prompt: []fantasy.Message{{
			Role:    fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "What is 17*23?"}},
		}},
	})
	require.NoError(t, err)

	var reasoning, text strings.Builder
	sawReasoningEnd := false
	for part := range parts {
		switch part.Type {
		case fantasy.StreamPartTypeReasoningDelta:
			_, _ = reasoning.WriteString(part.Delta)
		case fantasy.StreamPartTypeReasoningEnd:
			sawReasoningEnd = true
		case fantasy.StreamPartTypeTextDelta:
			_, _ = text.WriteString(part.Delta)
		}
	}

	require.Equal(t, "**Calculating**\n\nStep one.More thinking.", reasoning.String())
	require.True(t, sawReasoningEnd)
	require.Equal(t, "The answer is 391.", text.String())
}

type sseChatCompletionTransport struct {
	stream string
}

func (st *sseChatCompletionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	_, _ = io.ReadAll(req.Body)
	_ = req.Body.Close()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(st.stream)),
	}, nil
}
