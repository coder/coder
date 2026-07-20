package agenthooks_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/agenthooks"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestSignClaimsVerify(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", agenthooks.EventPreToolUse, nil)
	token, err := agenthooks.SignClaims(testSecret, claims)
	require.NoError(t, err)

	got, err := agenthooks.Verify("Bearer "+token, testSecret)
	require.NoError(t, err)
	require.Equal(t, claims, got)
}

func TestVerifyRejectsAlgorithmConfusion(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", agenthooks.EventStop, nil)
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS512, Key: bytes.Repeat([]byte{1}, 64)},
		new(jose.SignerOptions).WithType("JWT"),
	)
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	signed, err := signer.Sign(payload)
	require.NoError(t, err)
	token, err := signed.CompactSerialize()
	require.NoError(t, err)

	_, err = agenthooks.Verify("Bearer "+token, testSecret)
	require.Error(t, err)
}

func TestVerifyTimeBounds(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name   string
		update func(*agenthooks.Claims)
	}{
		{
			name: "expired",
			update: func(claims *agenthooks.Claims) {
				claims.Expires = now.Add(-time.Minute).Unix()
			},
		},
		{
			name: "not before",
			update: func(claims *agenthooks.Claims) {
				claims.NotBefore = now.Add(time.Minute).Unix()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims(t, "https://hooks.example.com/coder", agenthooks.EventStop, nil)
			test.update(&claims)
			token, err := agenthooks.SignClaims(testSecret, claims)
			require.NoError(t, err)

			_, err = agenthooks.Verify("Bearer "+token, testSecret)
			require.Error(t, err)
		})
	}
}

func TestHTTPHandlerRoutesEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   agenthooks.EventType
		data    any
		install func(*agenthooks.Hooks, *bool)
	}{
		{
			name:  "session start",
			event: agenthooks.EventSessionStart,
			data:  agenthooks.SessionStartData{Source: "startup"},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.SessionStart = func(_ context.Context, _ agenthooks.Meta, data agenthooks.SessionStartData) (agenthooks.Response, error) {
					*called = true
					require.Equal(t, "startup", data.Source)
					return agenthooks.Response{UserMessage: "session start"}, nil
				}
			},
		},
		{
			name:  "user prompt submit",
			event: agenthooks.EventUserPromptSubmit,
			data:  agenthooks.UserPromptSubmitData{Prompt: "hello"},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.UserPromptSubmit = func(_ context.Context, _ agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
					*called = true
					require.Equal(t, "hello", data.Prompt)
					return agenthooks.Response{UserMessage: "user prompt submit"}, nil
				}
			},
		},
		{
			name:  "pre tool use",
			event: agenthooks.EventPreToolUse,
			data: agenthooks.PreToolUseData{
				ToolUseID: "call_" + uuid.NewString(),
				ToolName:  "execute",
				ToolInput: json.RawMessage(`{"command":"pwd"}`),
			},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.PreToolUse = func(_ context.Context, _ agenthooks.Meta, data agenthooks.PreToolUseData) (agenthooks.Response, error) {
					*called = true
					require.Equal(t, "execute", data.ToolName)
					return agenthooks.Response{UserMessage: "pre tool use"}, nil
				}
			},
		},
		{
			name:  "post tool use",
			event: agenthooks.EventPostToolUse,
			data: agenthooks.PostToolUseData{
				ToolUseID:    "call_" + uuid.NewString(),
				ToolName:     "execute",
				ToolResponse: json.RawMessage(`{"output":"ok"}`),
			},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.PostToolUse = func(_ context.Context, _ agenthooks.Meta, data agenthooks.PostToolUseData) (agenthooks.Response, error) {
					*called = true
					require.Equal(t, "execute", data.ToolName)
					return agenthooks.Response{UserMessage: "post tool use"}, nil
				}
			},
		},
		{
			name:  "pre compact",
			event: agenthooks.EventPreCompact,
			data:  agenthooks.PreCompactData{},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.PreCompact = func(context.Context, agenthooks.Meta, agenthooks.PreCompactData) (agenthooks.Response, error) {
					*called = true
					return agenthooks.Response{UserMessage: "pre compact"}, nil
				}
			},
		},
		{
			name:  "post compact",
			event: agenthooks.EventPostCompact,
			data:  agenthooks.PostCompactData{},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.PostCompact = func(context.Context, agenthooks.Meta, agenthooks.PostCompactData) (agenthooks.Response, error) {
					*called = true
					return agenthooks.Response{UserMessage: "post compact"}, nil
				}
			},
		},
		{
			name:  "stop",
			event: agenthooks.EventStop,
			data:  agenthooks.StopData{},
			install: func(hooks *agenthooks.Hooks, called *bool) {
				hooks.Stop = func(context.Context, agenthooks.Meta, agenthooks.StopData) (agenthooks.Response, error) {
					*called = true
					return agenthooks.Response{UserMessage: "stop"}, nil
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			called := false
			var hooks agenthooks.Hooks
			test.install(&hooks, &called)
			server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, hooks))
			t.Cleanup(server.Close)

			response := postEvent(t, server.URL, test.event, test.data, nil, nil)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var got agenthooks.Response
			require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
			require.Equal(t, test.name, got.UserMessage)
			require.True(t, called)
		})
	}
}

func TestHTTPHandlerNoOpHookDoesNotDecodeData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, agenthooks.EventStop, "unused", nil, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var got agenthooks.Response
	require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
	require.Equal(t, agenthooks.Response{}, got)
}

func TestHTTPHandlerRejectsMismatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		updateRequest func(*agenthooks.Request)
		updateClaims  func(*agenthooks.Claims)
	}{
		{
			name: "dispatch ID",
			updateRequest: func(request *agenthooks.Request) {
				request.Meta.DispatchID = uuid.New()
			},
		},
		{
			name: "event type",
			updateRequest: func(request *agenthooks.Request) {
				request.Type = agenthooks.EventPreCompact
			},
		},
		{
			name: "chat ID",
			updateRequest: func(request *agenthooks.Request) {
				request.Meta.ChatID = uuid.New()
			},
		},
		{
			name: "audience",
			updateClaims: func(claims *agenthooks.Claims) {
				claims.Audience = "https://hooks.example.com/other"
			},
		},
		{
			name: "body SHA-256",
			updateClaims: func(claims *agenthooks.Claims) {
				claims.BodySHA256 = hex.EncodeToString(bytes.Repeat([]byte{1}, sha256.Size))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, agenthooks.Hooks{}))
			t.Cleanup(server.Close)
			response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, test.updateRequest, test.updateClaims)
			defer response.Body.Close()
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
		})
	}
}

func TestResponseAllowedToolsWireShape(t *testing.T) {
	t.Parallel()

	unset, err := json.Marshal(agenthooks.Response{})
	require.NoError(t, err)
	require.NotContains(t, string(unset), "allowed_tools")

	restrictAll, err := json.Marshal(agenthooks.Response{AllowedTools: &[]string{}})
	require.NoError(t, err)
	require.Contains(t, string(restrictAll), `"allowed_tools":[]`)

	var decoded agenthooks.Response
	require.NoError(t, json.Unmarshal([]byte(`{"allowed_tools":null}`), &decoded))
	require.Nil(t, decoded.AllowedTools)
	require.NoError(t, json.Unmarshal([]byte(`{"allowed_tools":[]}`), &decoded))
	require.NotNil(t, decoded.AllowedTools)
	require.Empty(t, *decoded.AllowedTools)
}

func TestHTTPHandlerAcceptsTrailingSlashAudience(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Audience = server.URL + "/"
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHTTPHandlerHonorsForwardedProto(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	httpsAudience := "https" + strings.TrimPrefix(server.URL, "http")
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Audience = httpsAudience
	}, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https, http")
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHTTPHandlerHonorsForwardedHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Audience = "https://hooks.example.com"
	}, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Host", "hooks.example.com, internal-lb")
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func postEvent(t *testing.T, target string, eventType agenthooks.EventType, data any, updateRequest func(*agenthooks.Request), updateClaims func(*agenthooks.Claims), updateHTTPRequest ...func(*http.Request)) *http.Response {
	t.Helper()

	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	request := agenthooks.Request{
		Type: eventType,
		Meta: agenthooks.Meta{
			DispatchID:    uuid.New(),
			SchemaVersion: agenthooks.SchemaVersion,
			ChatID:        uuid.New(),
			OwnerID:       uuid.New(),
		},
		Data: dataJSON,
	}
	claims := validClaims(t, target, eventType, &request)
	if updateRequest != nil {
		updateRequest(&request)
	}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	claims.BodySHA256 = hex.EncodeToString(digest[:])
	if updateClaims != nil {
		updateClaims(&claims)
	}
	token, err := agenthooks.SignClaims(testSecret, claims)
	require.NoError(t, err)
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	for _, update := range updateHTTPRequest {
		update(httpRequest)
	}
	response, err := http.DefaultClient.Do(httpRequest)
	require.NoError(t, err)
	return response
}

func validClaims(t *testing.T, audience string, eventType agenthooks.EventType, request *agenthooks.Request) agenthooks.Claims {
	t.Helper()

	now := time.Now()
	claims := agenthooks.Claims{
		Issuer:     uuid.NewString(),
		Subject:    "coder:chat:" + uuid.NewString(),
		Audience:   audience,
		IssuedAt:   now.Unix(),
		NotBefore:  now.Add(-time.Second).Unix(),
		Expires:    now.Add(time.Minute).Unix(),
		JTI:        uuid.New(),
		Type:       eventType,
		BodySHA256: hex.EncodeToString(make([]byte, sha256.Size)),
	}
	if request != nil {
		claims.Subject = "coder:chat:" + request.Meta.ChatID.String()
		claims.JTI = request.Meta.DispatchID
	}
	return claims
}
