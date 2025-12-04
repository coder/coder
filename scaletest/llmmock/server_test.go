package llmmock_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/scaletest/llmmock"
	"github.com/coder/coder/v2/testutil"
)

func TestServer_StartStop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	require.NotEmpty(t, srv.APIAddress())

	err = srv.Stop()
	require.NoError(t, err)
}

func TestServer_OpenAIRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	reqBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello, world!",
			},
		},
		"stream": false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token-12345")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var openAIResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&openAIResp)
	require.NoError(t, err)
	require.Equal(t, "chat.completion", openAIResp["object"])

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Query stored requests
	apiURL := fmt.Sprintf("%s/api/requests", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, llmmock.ProviderOpenAI, records[0].Request.Provider)
	require.Equal(t, "gpt-4", records[0].Request.Model)
	require.Equal(t, false, records[0].Request.Stream)
	require.NotNil(t, records[0].Response)
	require.Equal(t, "stop", records[0].Response.FinishReason)
}

func TestServer_AnthropicRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	reqBody := map[string]interface{}{
		"model": "claude-3-opus-20240229",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello, world!",
			},
		},
		"max_tokens": 1024,
		"stream":     false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/v1/messages", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token-67890")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var anthropicResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&anthropicResp)
	require.NoError(t, err)
	require.Equal(t, "message", anthropicResp["type"])

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Query stored requests
	apiURL := fmt.Sprintf("%s/api/requests?provider=anthropic", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, llmmock.ProviderAnthropic, records[0].Request.Provider)
	require.Equal(t, "claude-3-opus-20240229", records[0].Request.Model)
	require.Equal(t, false, records[0].Request.Stream)
	require.NotNil(t, records[0].Response)
	require.Equal(t, "end_turn", records[0].Response.FinishReason)
}

func TestServer_OpenAIStreaming(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	reqBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"stream": true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read streaming response
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err)
	content := string(buf[:n])
	require.Contains(t, content, "data:")
	require.Contains(t, content, "chat.completion.chunk")

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Verify stored request has stream flag
	apiURL := fmt.Sprintf("%s/api/requests", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, true, records[0].Request.Stream)
	require.Equal(t, true, records[0].Response.Stream)
}

func TestServer_AnthropicStreaming(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	reqBody := map[string]interface{}{
		"model": "claude-3-opus-20240229",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello!",
			},
		},
		"max_tokens": 1024,
		"stream":     true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	url := fmt.Sprintf("%s/v1/messages", srv.APIAddress())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read streaming response
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err)
	content := string(buf[:n])
	require.Contains(t, content, "data:")
	require.Contains(t, content, "message_start")

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Verify stored request has stream flag
	apiURL := fmt.Sprintf("%s/api/requests?provider=anthropic", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, true, records[0].Request.Stream)
	require.Equal(t, true, records[0].Response.Stream)
}

func TestServer_FilterByUserID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	// Send request with user token 1
	reqBody1 := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes1, _ := json.Marshal(reqBody1)
	url1 := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req1, _ := http.NewRequestWithContext(ctx, http.MethodPost, url1, bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer user-token-12345")
	_, _ = http.DefaultClient.Do(req1)

	// Send request with user token 2
	reqBody2 := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "World"},
		},
	}
	bodyBytes2, _ := json.Marshal(reqBody2)
	url2 := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, url2, bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer user-token-67890")
	_, _ = http.DefaultClient.Do(req2)

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 2
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Filter by user_id (first 8 chars of token)
	apiURL := fmt.Sprintf("%s/api/requests?user_id=user-tok", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.True(t, strings.HasPrefix(records[0].Request.UserID, "user-tok"))
}

func TestServer_FilterByProvider(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	// Send OpenAI request
	reqBody1 := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes1, _ := json.Marshal(reqBody1)
	url1 := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req1, _ := http.NewRequestWithContext(ctx, http.MethodPost, url1, bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(req1)

	// Send Anthropic request
	reqBody2 := map[string]interface{}{
		"model": "claude-3-opus-20240229",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "World"},
		},
		"max_tokens": 1024,
	}
	bodyBytes2, _ := json.Marshal(reqBody2)
	url2 := fmt.Sprintf("%s/v1/messages", srv.APIAddress())
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, url2, bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("anthropic-version", "2023-06-01")
	_, _ = http.DefaultClient.Do(req2)

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 2
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Filter by provider
	apiURL := fmt.Sprintf("%s/api/requests?provider=openai", srv.APIAddress())
	apiReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	require.NoError(t, err)

	apiResp, err := http.DefaultClient.Do(apiReq)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	var records []llmmock.RequestRecord
	err = json.NewDecoder(apiResp.Body).Decode(&records)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, llmmock.ProviderOpenAI, records[0].Request.Provider)
}

func TestServer_Purge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := new(llmmock.Server)
	err := srv.Start(ctx, llmmock.Config{
		HostAddress: "127.0.0.1",
		APIPort:     0,
		Logger:      slogtest.Make(t, nil),
	})
	require.NoError(t, err)
	defer srv.Stop()

	// Send a request
	reqBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/v1/chat/completions", srv.APIAddress())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(req)

	require.Eventually(t, func() bool {
		return srv.RequestCount() == 1
	}, testutil.WaitShort, testutil.IntervalMedium)

	// Purge
	purgeURL := fmt.Sprintf("%s/api/purge", srv.APIAddress())
	purgeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, purgeURL, nil)
	require.NoError(t, err)

	purgeResp, err := http.DefaultClient.Do(purgeReq)
	require.NoError(t, err)
	defer purgeResp.Body.Close()
	require.Equal(t, http.StatusOK, purgeResp.StatusCode)

	require.Equal(t, 0, srv.RequestCount())
}
