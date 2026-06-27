package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
)

// TestBuildClaudePlatformAWSOptionsValidation covers the input validation that
// does not require resolving credentials.
func TestBuildClaudePlatformAWSOptionsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.AWSClaudePlatform
		errorMsg string
	}{
		{
			name:     "missing workspace id",
			cfg:      config.AWSClaudePlatform{Region: "us-east-1"},
			errorMsg: "workspace id required",
		},
		{
			name:     "missing region and base url",
			cfg:      config.AWSClaudePlatform{WorkspaceID: "wrkspc_123"},
			errorMsg: "region or base url required",
		},
		{
			name: "missing access key secret",
			cfg: config.AWSClaudePlatform{
				Region:      "us-east-1",
				WorkspaceID: "wrkspc_123",
				AccessKey:   "AKIAEXAMPLE",
			},
			errorMsg: "both access key and access key secret must be provided together",
		},
		{
			name: "missing access key",
			cfg: config.AWSClaudePlatform{
				Region:          "us-east-1",
				WorkspaceID:     "wrkspc_123",
				AccessKeySecret: "secret",
			},
			errorMsg: "both access key and access key secret must be provided together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := buildClaudePlatformAWSOptions(context.Background(), tt.cfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

type capturedClaudePlatformRequest struct {
	headers http.Header
	url     string
	body    []byte
}

// newClaudePlatformTestServer returns an httptest server that records the
// inbound request and replies with a minimal valid Anthropic Messages response.
func newClaudePlatformTestServer(t *testing.T, captured *capturedClaudePlatformRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured.headers = r.Header.Clone()
		captured.url = r.URL.String()
		captured.body = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg_test",
			"type":          "message",
			"role":          "assistant",
			"content":       []map[string]any{{"type": "text", "text": "hi"}},
			"model":         "claude-sonnet-4-5",
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestClaudePlatformAWSSigV4 verifies that with static AWS credentials, requests
// to Claude Platform for AWS are SigV4-signed for the aws-external-anthropic
// service, carry the anthropic-workspace-id header, and pass the native
// Anthropic model ID through unchanged.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestClaudePlatformAWSSigV4(t *testing.T) {
	t.Setenv("ANTHROPIC_AWS_API_KEY", "")
	t.Setenv("ANTHROPIC_AWS_WORKSPACE_ID", "")
	t.Setenv("ANTHROPIC_AWS_BASE_URL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var captured capturedClaudePlatformRequest
	srv := newClaudePlatformTestServer(t, &captured)

	opts, err := buildClaudePlatformAWSOptions(context.Background(), config.AWSClaudePlatform{
		Region:          "us-east-1",
		WorkspaceID:     "wrkspc_123",
		AccessKey:       "AKIAEXAMPLE",
		AccessKeySecret: "secret",
		BaseURL:         srv.URL,
	})
	require.NoError(t, err)

	svc := anthropic.NewMessageService(opts...)
	_, err = svc.New(context.Background(), anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
	})
	require.NoError(t, err)

	auth := captured.headers.Get("Authorization")
	require.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256"), "expected SigV4 Authorization, got %q", auth)
	require.Contains(t, auth, "/aws-external-anthropic/", "expected aws-external-anthropic service in credential scope")
	require.NotEmpty(t, captured.headers.Get("X-Amz-Date"))
	require.Equal(t, "wrkspc_123", captured.headers.Get("Anthropic-Workspace-Id"))
	// SigV4 mode must not send an API key.
	require.Empty(t, captured.headers.Get("X-Api-Key"))
	require.Equal(t, "/v1/messages", strings.Split(captured.url, "?")[0])

	// Model passes through unchanged (no Bedrock-style rewriting).
	var sentBody struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(captured.body, &sentBody))
	require.Equal(t, "claude-sonnet-4-5", sentBody.Model)
}

// TestClaudePlatformAWSAPIKey verifies that when an API key is configured,
// requests authenticate with x-api-key (not SigV4) while still carrying the
// anthropic-workspace-id header.
// NOTE: no t.Parallel() because it uses t.Setenv.
func TestClaudePlatformAWSAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_AWS_API_KEY", "")
	t.Setenv("ANTHROPIC_AWS_WORKSPACE_ID", "")
	t.Setenv("ANTHROPIC_AWS_BASE_URL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var captured capturedClaudePlatformRequest
	srv := newClaudePlatformTestServer(t, &captured)

	opts, err := buildClaudePlatformAWSOptions(context.Background(), config.AWSClaudePlatform{
		Region:      "us-east-1",
		WorkspaceID: "wrkspc_123",
		APIKey:      "sk-ant-workspace-key",
		BaseURL:     srv.URL,
	})
	require.NoError(t, err)

	svc := anthropic.NewMessageService(opts...)
	_, err = svc.New(context.Background(), anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
	})
	require.NoError(t, err)

	require.Equal(t, "sk-ant-workspace-key", captured.headers.Get("X-Api-Key"))
	require.Equal(t, "wrkspc_123", captured.headers.Get("Anthropic-Workspace-Id"))
	require.Empty(t, captured.headers.Get("Authorization"), "API key mode must not SigV4-sign")
}
