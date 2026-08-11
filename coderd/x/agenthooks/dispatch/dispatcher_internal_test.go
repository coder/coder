package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

const (
	testSecret       = "test-hook-secret-32-bytes-minimum!!"
	testDeploymentID = "test-deployment"
	testVersion      = "test-version"
)

func TestDispatcherSuccess(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventSessionStart, agenthooks.SessionStartData{Source: "new"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "coderd-agenthooks/"+testVersion, r.Header.Get("User-Agent"))

		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(testSecret))
		assert.NoError(t, err)
		assert.Equal(t, testDeploymentID, claims.Issuer)
		assert.Equal(t, serverURL(r), claims.Audience)
		assert.Equal(t, event.Type, claims.Type)
		chatID, err := claims.ChatID()
		assert.NoError(t, err)
		assert.Equal(t, event.ChatID, chatID)
		digest := sha256.Sum256(body)
		assert.Equal(t, hex.EncodeToString(digest[:]), claims.BodySHA256)
		assert.Equal(t, claims.IssuedAt-int64(clockSkewLeeway/time.Second), claims.NotBefore)

		var request agenthooks.Request
		assert.NoError(t, json.Unmarshal(body, &request))
		assert.Equal(t, claims.JTI, request.Meta.DispatchID)
		assert.Equal(t, agenthooks.SchemaVersion, request.Meta.SchemaVersion)
		var decoded agenthooks.SessionStartData
		assert.NoError(t, json.Unmarshal(request.Data, &decoded))
		assert.Equal(t, agenthooks.SessionStartData{Source: "new"}, decoded)

		_, err = w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	dispatcher := newTestDispatcher(t, server.Client(), server.URL, 2*time.Second)
	response, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
	require.NoError(t, err)
	require.Equal(t, agenthooks.Response{}, response)
}

func TestDispatcherRejectsCleartextURL(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{ToolUseID: "call_1", ToolName: "execute"})
	dispatcher := newTestDispatcher(t, nil, "http://hooks.example.com/coder", 2*time.Second)
	_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitShort), event)
	require.ErrorContains(t, err, "must use HTTPS")

	require.NoError(t, validateHookURL("", false))
	require.NoError(t, validateHookURL("https://hooks.example.com/coder", false))
	require.NoError(t, validateHookURL("http://localhost:8080/hooks", false))
	require.NoError(t, validateHookURL("http://127.0.0.1:8080/hooks", false))
	require.NoError(t, validateHookURL("http://[::1]:8080/hooks", false))
	require.Error(t, validateHookURL("http://10.0.0.5/hooks", false))
	require.Error(t, validateHookURL("ftp://hooks.example.com/coder", false))
	require.ErrorContains(t, validateHookURL("https:///coder", false), "must include a host")
	require.ErrorContains(t, validateHookURL("https:hooks.example.com", false), "must include a host")
	require.ErrorContains(t, validateHookURL("http:///hooks", false), "must include a host")
	require.ErrorContains(t, validateHookURL("https://hooks.example.com/coder#frag", false), "must not contain a fragment")
	require.ErrorContains(t, validateHookURL("https://user:pass@hooks.example.com/coder", false), "must not contain userinfo")

	require.NoError(t, validateHookURL("http://10.0.0.5/hooks", true))
	require.NoError(t, validateHookURL("http://hooks.example.com/coder", true))
	require.Error(t, validateHookURL("ftp://hooks.example.com/coder", true))
	require.ErrorContains(t, validateHookURL("http:///hooks", true), "must include a host")
	require.ErrorContains(t, validateHookURL("http://hooks.example.com/coder#frag", true), "must not contain a fragment")
	require.ErrorContains(t, validateHookURL("http://user:pass@hooks.example.com/coder", true), "must not contain userinfo")
}

func TestDispatcherDeny(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{Prompt: "delete everything"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"deny","reason":"blocked"},"user_message":"not allowed"}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.Equal(t, agenthooks.PermissionDeny, response.Permission.Decision)
	require.Equal(t, "not allowed", response.UserMessage)
}

func TestDispatcherDenyNullOverrideDecode(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: "tool-use-1",
		ToolName:  "execute",
		ToolInput: json.RawMessage(`{"cmd":"rm"}`),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"deny","input_override":null}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.Equal(t, agenthooks.PermissionDeny, response.Permission.Decision)
	require.JSONEq(t, `null`, string(response.Permission.InputOverride))
}

func TestDispatcherAllowInputOverride(t *testing.T) {
	t.Parallel()

	toolInput := json.RawMessage(`{"path":"before"}`)
	toolUseID := "call_" + uuid.NewString()
	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolUseID,
		ToolName:  "edit",
		ToolInput: toolInput,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"path":"after"}}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.Equal(t, agenthooks.PermissionAllow, response.Permission.Decision)
	require.JSONEq(t, `{"path":"after"}`, string(response.Permission.InputOverride))
}

func TestDispatcherAllowsCaseDistinctOverrideKeys(t *testing.T) {
	t.Parallel()

	// Tool schemas are case-sensitive, so "URL" and "url" are distinct
	// properties even though the response envelope folds its own keys.
	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: "call_" + uuid.NewString(),
		ToolName:  "fetch",
		ToolInput: json.RawMessage(`{"url":"before"}`),
	})
	// The envelope key is folded by the decoder, so the boundary detection
	// must fold too or the case-distinct override below is wrongly rejected.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","INPUT_OVERRIDE":{"URL":"upper","url":"lower"}}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.JSONEq(t, `{"URL":"upper","url":"lower"}`, string(response.Permission.InputOverride))
}

func TestDispatcherTimeoutNoRetry(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		assert.NoError(t, http.NewResponseController(w).Flush())
		<-release
	}))
	t.Cleanup(server.Close)

	_, _, err := newTestDispatcher(t, server.Client(), server.URL, 50*time.Millisecond).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	close(release)
	assertDispatchErrorClass(t, err, ResultTimeout)
	require.Equal(t, int32(1), requests.Load())
}

func TestDispatcherRetriesConnectionErrorWithSameJTI(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPostCompact, agenthooks.PostCompactData{})
	claimsCh := make(chan agenthooks.Claims, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(testSecret))
		assert.NoError(t, err)
		claimsCh <- claims
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var attempts atomic.Int32
	baseTransport := server.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		claims, err := agenthooks.Verify(req.Header.Get("Authorization"), []byte(testSecret))
		if err != nil {
			return nil, err
		}
		if attempts.Add(1) == 1 {
			claimsCh <- claims
			_, err = io.Copy(io.Discard, req.Body)
			if err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		return baseTransport.RoundTrip(req)
	})}

	_, dispatchID, err := newTestDispatcher(t, client, server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
	first := <-claimsCh
	second := <-claimsCh
	require.Equal(t, first.JTI, second.JTI)
	require.Equal(t, first.JTI, dispatchID)
	chatID, err := first.ChatID()
	require.NoError(t, err)
	require.Equal(t, event.ChatID, chatID)
}

func TestDispatcherRetriesHTTP2AbortWithSameDispatchID(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPostCompact, agenthooks.PostCompactData{})
	type attemptIdentity struct {
		jti        uuid.UUID
		dispatchID uuid.UUID
	}
	identities := make(chan attemptIdentity, 2)
	var attempts atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, 2, r.ProtoMajor)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(testSecret))
		assert.NoError(t, err)
		var request agenthooks.Request
		assert.NoError(t, json.Unmarshal(body, &request))
		identities <- attemptIdentity{jti: claims.JTI, dispatchID: request.Meta.DispatchID}

		if attempts.Add(1) == 1 {
			_, err = w.Write([]byte(`{"partial":`))
			assert.NoError(t, err)
			assert.NoError(t, http.NewResponseController(w).Flush())
			panic(http.ErrAbortHandler)
		}
		_, err = w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	_, dispatchID, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
	first := <-identities
	second := <-identities
	require.Equal(t, first, second)
	require.Equal(t, dispatchID, first.jti)
	require.Equal(t, dispatchID, first.dispatchID)
}

func TestDispatcherRetriesMidBodyConnectionError(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPostCompact, agenthooks.PostCompactData{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var attempts atomic.Int32
	baseTransport := server.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			_, err := io.Copy(io.Discard, req.Body)
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(errReader{err: io.ErrUnexpectedEOF}),
				Header:     http.Header{},
			}, nil
		}
		return baseTransport.RoundTrip(req)
	})}

	_, _, err := newTestDispatcher(t, client, server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestDispatcherTLSFailureNoRetry(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request with an untrusted certificate reached the handler")
	}))
	t.Cleanup(server.Close)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return transport.RoundTrip(req)
	})}

	_, _, err := newTestDispatcher(t, client, server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	assertDispatchErrorClass(t, err, ResultProtocolError)
	require.Equal(t, int32(1), attempts.Load())
}

func TestDispatcherNon2xxNoRetry(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPreCompact, agenthooks.PreCompactData{})
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		assert.NoError(t, http.NewResponseController(w).Flush())
		<-release
	}))
	t.Cleanup(server.Close)

	_, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	close(release)
	assertDispatchErrorClass(t, err, ResultHTTPError)
	require.Equal(t, int32(1), requests.Load())
}

func TestDispatcherProtocolErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		eventType    agenthooks.EventType
		data         any
		responseBody []byte
	}{
		{
			name:         "malformed JSON",
			eventType:    agenthooks.EventStop,
			data:         agenthooks.StopData{},
			responseBody: []byte(`{"user_message":`),
		},
		{
			name:         "misspelled permission key",
			eventType:    agenthooks.EventPreToolUse,
			data:         agenthooks.PreToolUseData{ToolUseID: "call_typo", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: []byte(`{"permision":{"decision":"deny"}}`),
		},
		{
			name:      "folded duplicate in prompt override",
			eventType: agenthooks.EventUserPromptSubmit,
			data:      agenthooks.UserPromptSubmitData{Prompt: "original"},
			// This override is struct-decoded, so folding applies to it.
			responseBody: []byte(`{"permission":{"decision":"allow","input_override":{"prompt":"approved","Prompt":"different"}}}`),
		},
		{
			name:      "duplicate key inside input_override",
			eventType: agenthooks.EventPreToolUse,
			data:      agenthooks.PreToolUseData{ToolUseID: "call_dup_override", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			// Exact duplicates stay rejected at any depth.
			responseBody: []byte(`{"permission":{"decision":"allow","input_override":{"url":"a","url":"b"}}}`),
		},
		{
			name:      "unicode-folded duplicate permission key",
			eventType: agenthooks.EventPreToolUse,
			data:      agenthooks.PreToolUseData{ToolUseID: "call_unicode", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			// encoding/json folds U+017F to "s", so this aliases "permission".
			responseBody: []byte(`{"permission":{"decision":"deny"},"permiſſion":null}`),
		},
		{
			name:      "case-folded duplicate permission key",
			eventType: agenthooks.EventPreToolUse,
			data:      agenthooks.PreToolUseData{ToolUseID: "call_fold", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			// encoding/json matches fields case-insensitively and keeps the
			// last value, so this would otherwise decode as an empty allow.
			responseBody: []byte(`{"permission":{"decision":"deny"},"Permission":null}`),
		},
		{
			name:         "duplicate permission key",
			eventType:    agenthooks.EventPreToolUse,
			data:         agenthooks.PreToolUseData{ToolUseID: "call_dup", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: []byte(`{"permission":{"decision":"deny"},"permission":null}`),
		},
		{
			name:         "unknown permission field",
			eventType:    agenthooks.EventPreToolUse,
			data:         agenthooks.PreToolUseData{ToolUseID: "call_unknown", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: []byte(`{"permission":{"decision":"deny","reasoning":"typo"}}`),
		},
		{
			name:         "duplicate key inside input_override",
			eventType:    agenthooks.EventPreToolUse,
			data:         agenthooks.PreToolUseData{ToolUseID: "call_override_dup", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: []byte(`{"permission":{"decision":"allow","input_override":{"cmd":"ls","cmd":"rm -rf"}}}`),
		},
		{
			name:         "trailing JSON value",
			eventType:    agenthooks.EventStop,
			data:         agenthooks.StopData{},
			responseBody: []byte(`{"user_message":"done"}{}`),
		},
		{
			name:         "oversized model context",
			eventType:    agenthooks.EventStop,
			data:         agenthooks.StopData{},
			responseBody: mustJSON(t, agenthooks.Response{ModelContext: string(bytes.Repeat([]byte("x"), maxModelContextBytes+1))}),
		},
		{
			name:      "invalid user prompt override shape",
			eventType: agenthooks.EventUserPromptSubmit,
			data:      agenthooks.UserPromptSubmitData{Prompt: "question"},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision:      agenthooks.PermissionAllow,
				InputOverride: json.RawMessage(`{"unexpected":"value"}`),
			}}),
		},
		{
			name:      "deny with input override",
			eventType: agenthooks.EventPreToolUse,
			data: agenthooks.PreToolUseData{
				ToolUseID: "call_deny_override",
				ToolName:  "edit",
				ToolInput: json.RawMessage(`{"path":"a"}`),
			},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision:      agenthooks.PermissionDeny,
				InputOverride: json.RawMessage(`{"path":"b"}`),
			}}),
		},
		{
			name:      "unsupported ask decision",
			eventType: agenthooks.EventUserPromptSubmit,
			data:      agenthooks.UserPromptSubmitData{Prompt: "question"},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision: agenthooks.PermissionDecision("ask"),
			}}),
		},
		{
			name:      "pre_tool_use allow without input_override",
			eventType: agenthooks.EventPreToolUse,
			data:      agenthooks.PreToolUseData{ToolUseID: "call_no_override", ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision: agenthooks.PermissionAllow,
			}}),
		},
		{
			name:         "pre_tool_use without tool_use_id",
			eventType:    agenthooks.EventPreToolUse,
			data:         agenthooks.PreToolUseData{ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: []byte(`{}`),
		},
		{
			name:         "post_tool_use without tool_name",
			eventType:    agenthooks.EventPostToolUse,
			data:         agenthooks.PostToolUseData{ToolUseID: "call_no_name"},
			responseBody: []byte(`{}`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			event := newTestEvent(t, test.eventType, test.data)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write(test.responseBody)
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)

			_, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
				testutil.Context(t, testutil.WaitLong), event,
			)
			assertDispatchErrorClass(t, err, ResultProtocolError)
		})
	}
}

func TestDispatcherOverCapacity(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	dispatcher := newTestDispatcher(t, nil, "https://unused.test", 10*time.Millisecond)
	for range maxConcurrentDispatches {
		dispatcher.semaphore <- struct{}{}
	}
	defer func() {
		for range maxConcurrentDispatches {
			<-dispatcher.semaphore
		}
	}()

	_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assertDispatchErrorClass(t, err, ResultOverCapacity)
}

func TestDispatcherRejectedResponseIsNotObserved(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: "call_" + uuid.NewString(),
		ToolName:  "execute",
		ToolInput: json.RawMessage(`{"cmd":"ls"}`),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"deny","input_override":{"cmd":"rm"}},"model_context":"ctx"}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	registry := prometheus.NewRegistry()
	dispatcher := New(
		testutil.Logger(t), server.Client(), server.URL, false, testSecret, time.Second,
		testDeploymentID, testVersion, registry,
	)
	_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
	assertDispatchErrorClass(t, err, ResultProtocolError)

	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		switch family.GetName() {
		case "coderd_chatd_hook_decisions_total",
			"coderd_chatd_hook_input_overrides_total",
			"coderd_chatd_hook_context_size_bytes":
			require.Empty(t, family.GetMetric(), "rejected response observed in %s", family.GetName())
		}
	}
}

func TestDispatcherCanceledContextAtCapacityReturnsTimeout(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	dispatcher := newTestDispatcher(t, nil, "https://unused.test", testutil.WaitLong)
	for range maxConcurrentDispatches {
		dispatcher.semaphore <- struct{}{}
	}
	defer func() {
		for range maxConcurrentDispatches {
			<-dispatcher.semaphore
		}
	}()

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	cancel()
	_, _, err := dispatcher.Dispatch(ctx, event)
	require.ErrorIs(t, err, context.Canceled)
	assertDispatchErrorClass(t, err, ResultTimeout)
}

func TestDispatcherCanceledContextReturnsTimeout(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
	cancel()
	_, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(ctx, event)
	assertDispatchErrorClass(t, err, ResultTimeout)
}

func TestDispatcherInvalidToolInputReturnsProtocolError(t *testing.T) {
	t.Parallel()

	toolUseID := "call_" + uuid.NewString()
	event := newTestEvent(t, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolUseID,
		ToolName:  "edit",
		ToolInput: json.RawMessage(`{"path":`),
	})

	var hookRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hookRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	_, _, err := newTestDispatcher(t, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	assertDispatchErrorClass(t, err, ResultProtocolError)
	require.Zero(t, hookRequests.Load())
}

func newTestDispatcher(
	t *testing.T,
	client *http.Client,
	hookURL string,
	timeout time.Duration,
) *Dispatcher {
	t.Helper()
	return New(
		testutil.Logger(t),
		client,
		hookURL,
		false,
		testSecret,
		timeout,
		testDeploymentID,
		testVersion,
		prometheus.NewRegistry(),
	)
}

func newTestEvent(t *testing.T, eventType agenthooks.EventType, data any) Event {
	t.Helper()
	return Event{
		Type: eventType,
		ChatRef: agenthooks.ChatRef{
			ChatID:  uuid.New(),
			OwnerID: uuid.New(),
		},
		Data:     data,
		Capacity: CapacityClassGeneration,
	}
}

func assertDispatchErrorClass(t *testing.T, err error, expected Result) {
	t.Helper()
	var dispatchErr *Error
	require.ErrorAs(t, err, &dispatchErr)
	require.Equal(t, expected, dispatchErr.Class)
	require.NotEqual(t, uuid.Nil, dispatchErr.DispatchID)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDispatcherCapacityClassRequired(t *testing.T) {
	t.Parallel()

	event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
	event.Capacity = CapacityClassUnset
	dispatcher := newTestDispatcher(t, nil, "https://unused.test", time.Second)

	_, dispatchID, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitShort), event)
	require.ErrorContains(t, err, "no capacity class")
	require.Equal(t, uuid.Nil, dispatchID)
}

func TestDispatcherAdmissionReserve(t *testing.T) {
	t.Parallel()

	fill := func(t *testing.T, pool chan struct{}, count int) {
		t.Helper()
		for range count {
			pool <- struct{}{}
		}
		t.Cleanup(func() {
			for range count {
				<-pool
			}
		})
	}

	t.Run("AdmissionReleasesBothPools", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{}`))
			assert.NoError(t, err)
		}))
		t.Cleanup(server.Close)

		dispatcher := New(
			testutil.Logger(t), server.Client(), server.URL, false, testSecret, testutil.WaitShort,
			testDeploymentID, testVersion, prometheus.NewRegistry(),
		)
		event := newTestEvent(t, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{Prompt: "hi"})
		event.Capacity = CapacityClassAdmission

		_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
		require.NoError(t, err)
		require.Empty(t, dispatcher.admission)
		require.Empty(t, dispatcher.semaphore)
	})

	t.Run("SaturatedAdmissionRefusesAdmission", func(t *testing.T) {
		t.Parallel()

		dispatcher := newTestDispatcher(t, nil, "https://unused.test", 10*time.Millisecond)
		fill(t, dispatcher.admission, maxAdmissionDispatches)

		event := newTestEvent(t, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{Prompt: "hi"})
		event.Capacity = CapacityClassAdmission
		_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
		assertDispatchErrorClass(t, err, ResultOverCapacity)
		require.Empty(t, dispatcher.semaphore, "a refused admission must not hold a shared slot")
	})

	t.Run("ExpiredDeadlineRefusesFreeSlot", func(t *testing.T) {
		t.Parallel()

		pool := make(chan struct{}, 1)
		release, outcome, ok := acquire(testutil.Context(t, testutil.WaitShort), pool, time.Now().Add(-time.Millisecond))
		require.False(t, ok)
		require.Nil(t, release)
		require.Equal(t, ResultOverCapacity, outcome.result)
		require.Empty(t, pool)
	})

	t.Run("RefusedSharedAcquireReleasesAdmission", func(t *testing.T) {
		t.Parallel()

		dispatcher := newTestDispatcher(t, nil, "https://unused.test", 10*time.Millisecond)
		fill(t, dispatcher.semaphore, maxConcurrentDispatches)

		event := newTestEvent(t, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{Prompt: "hi"})
		event.Capacity = CapacityClassAdmission
		_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
		assertDispatchErrorClass(t, err, ResultOverCapacity)
		require.Empty(t, dispatcher.admission, "an admission refused by the shared pool must release its gate token")
	})

	t.Run("SaturatedAdmissionStillServesGeneration", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{}`))
			assert.NoError(t, err)
		}))
		t.Cleanup(server.Close)

		dispatcher := New(
			testutil.Logger(t), server.Client(), server.URL, false, testSecret, testutil.WaitShort,
			testDeploymentID, testVersion, prometheus.NewRegistry(),
		)
		fill(t, dispatcher.admission, maxAdmissionDispatches)
		fill(t, dispatcher.semaphore, maxAdmissionDispatches)

		event := newTestEvent(t, agenthooks.EventStop, agenthooks.StopData{})
		_, _, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
		require.NoError(t, err)
	})
}
