package hooks_test

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

	"github.com/coder/coder/v2/coderd/x/hooks"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func TestSignClaimsVerify(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", hooks.EventPreToolUse, nil)
	token, err := hooks.SignClaims(testSecret, claims)
	require.NoError(t, err)

	got, err := hooks.Verify("Bearer "+token, testSecret)
	require.NoError(t, err)
	require.Equal(t, claims, got)
}

func TestShortSecretRejected(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", hooks.EventPreToolUse, nil)
	shortSecret := testSecret[:hooks.MinSecretLen-1]

	_, err := hooks.SignClaims(shortSecret, claims)
	require.ErrorContains(t, err, "secret must be at least")

	token, err := hooks.SignClaims(testSecret, claims)
	require.NoError(t, err)
	_, err = hooks.Verify("Bearer "+token, shortSecret)
	require.ErrorContains(t, err, "secret must be at least")
	_, err = hooks.Verify("Bearer "+token, nil)
	require.ErrorContains(t, err, "secret must be at least")
}

func TestVerifyRejectsAlgorithmConfusion(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", hooks.EventStop, nil)
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

	_, err = hooks.Verify("Bearer "+token, testSecret)
	require.Error(t, err)
}

func TestVerifyTimeBounds(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name   string
		update func(*hooks.Claims)
	}{
		{
			name: "expired",
			update: func(claims *hooks.Claims) {
				claims.Expires = now.Add(-time.Minute).Unix()
			},
		},
		{
			name: "not before",
			update: func(claims *hooks.Claims) {
				claims.NotBefore = now.Add(time.Minute).Unix()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims(t, "https://hooks.example.com/coder", hooks.EventStop, nil)
			test.update(&claims)
			token, err := hooks.SignClaims(testSecret, claims)
			require.NoError(t, err)

			_, err = hooks.Verify("Bearer "+token, testSecret)
			require.Error(t, err)
		})
	}
}

func TestHTTPHandlerRoutesEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   hooks.EventType
		data    any
		install func(*hooks.Hooks, *bool)
	}{
		{
			name:  "session start",
			event: hooks.EventSessionStart,
			data:  hooks.SessionStartData{Source: "startup"},
			install: func(h *hooks.Hooks, called *bool) {
				h.SessionStart = func(_ context.Context, _ hooks.Meta, data hooks.SessionStartData) (hooks.Response, error) {
					*called = true
					require.Equal(t, "startup", data.Source)
					return hooks.Response{UserMessage: "session start"}, nil
				}
			},
		},
		{
			name:  "user prompt submit",
			event: hooks.EventUserPromptSubmit,
			data:  hooks.UserPromptSubmitData{Prompt: "hello"},
			install: func(h *hooks.Hooks, called *bool) {
				h.UserPromptSubmit = func(_ context.Context, _ hooks.Meta, data hooks.UserPromptSubmitData) (hooks.Response, error) {
					*called = true
					require.Equal(t, "hello", data.Prompt)
					return hooks.Response{UserMessage: "user prompt submit"}, nil
				}
			},
		},
		{
			name:  "pre tool use",
			event: hooks.EventPreToolUse,
			data: hooks.PreToolUseData{
				ToolUseID: "call_" + uuid.NewString(),
				ToolName:  "execute",
				ToolInput: json.RawMessage(`{"command":"pwd"}`),
			},
			install: func(h *hooks.Hooks, called *bool) {
				h.PreToolUse = func(_ context.Context, _ hooks.Meta, data hooks.PreToolUseData) (hooks.Response, error) {
					*called = true
					require.Equal(t, "execute", data.ToolName)
					return hooks.Response{UserMessage: "pre tool use"}, nil
				}
			},
		},
		{
			name:  "post tool use",
			event: hooks.EventPostToolUse,
			data: hooks.PostToolUseData{
				ToolUseID:    "call_" + uuid.NewString(),
				ToolName:     "execute",
				ToolResponse: json.RawMessage(`{"output":"ok"}`),
			},
			install: func(h *hooks.Hooks, called *bool) {
				h.PostToolUse = func(_ context.Context, _ hooks.Meta, data hooks.PostToolUseData) (hooks.Response, error) {
					*called = true
					require.Equal(t, "execute", data.ToolName)
					return hooks.Response{UserMessage: "post tool use"}, nil
				}
			},
		},
		{
			name:  "pre compact",
			event: hooks.EventPreCompact,
			data:  hooks.PreCompactData{},
			install: func(h *hooks.Hooks, called *bool) {
				h.PreCompact = func(context.Context, hooks.Meta, hooks.PreCompactData) (hooks.Response, error) {
					*called = true
					return hooks.Response{UserMessage: "pre compact"}, nil
				}
			},
		},
		{
			name:  "post compact",
			event: hooks.EventPostCompact,
			data:  hooks.PostCompactData{},
			install: func(h *hooks.Hooks, called *bool) {
				h.PostCompact = func(context.Context, hooks.Meta, hooks.PostCompactData) (hooks.Response, error) {
					*called = true
					return hooks.Response{UserMessage: "post compact"}, nil
				}
			},
		},
		{
			name:  "stop",
			event: hooks.EventStop,
			data:  hooks.StopData{},
			install: func(h *hooks.Hooks, called *bool) {
				h.Stop = func(context.Context, hooks.Meta, hooks.StopData) (hooks.Response, error) {
					*called = true
					return hooks.Response{UserMessage: "stop"}, nil
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			called := false
			var h hooks.Hooks
			test.install(&h, &called)
			server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, h))
			t.Cleanup(server.Close)

			response := postEvent(t, server.URL, test.event, test.data, nil, nil)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var got hooks.Response
			require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
			require.Equal(t, test.name, got.UserMessage)
			require.True(t, called)
		})
	}
}

func TestHTTPHandlerNoOpHookDoesNotDecodeData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, hooks.EventStop, "unused", nil, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var got hooks.Response
	require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
	require.Equal(t, hooks.Response{}, got)
}

func TestHTTPHandlerRejectsMismatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		updateRequest func(*hooks.Request)
		updateClaims  func(*hooks.Claims)
	}{
		{
			name: "dispatch ID",
			updateRequest: func(request *hooks.Request) {
				request.Meta.DispatchID = uuid.New()
			},
		},
		{
			name: "event type",
			updateRequest: func(request *hooks.Request) {
				request.Type = hooks.EventPreCompact
			},
		},
		{
			name: "chat ID",
			updateRequest: func(request *hooks.Request) {
				request.Meta.ChatID = uuid.New()
			},
		},
		{
			name: "audience",
			updateClaims: func(claims *hooks.Claims) {
				claims.Audience = "https://hooks.example.com/other"
			},
		},
		{
			name: "body SHA-256",
			updateClaims: func(claims *hooks.Claims) {
				claims.BodySHA256 = hex.EncodeToString(bytes.Repeat([]byte{1}, sha256.Size))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
			t.Cleanup(server.Close)
			response := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, test.updateRequest, test.updateClaims)
			defer response.Body.Close()
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
		})
	}
}

func TestHTTPHandlerExpectedIssuer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(
		testSecret,
		hooks.Hooks{},
		hooks.WithExpectedIssuer("deployment-a"),
	))
	t.Cleanup(server.Close)

	matching := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, nil, func(claims *hooks.Claims) {
		claims.Issuer = "deployment-a"
	})
	defer matching.Body.Close()
	require.Equal(t, http.StatusOK, matching.StatusCode)

	mismatched := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, nil, func(claims *hooks.Claims) {
		claims.Issuer = "deployment-b"
	})
	defer mismatched.Body.Close()
	require.Equal(t, http.StatusUnauthorized, mismatched.StatusCode)
}

func TestHTTPHandlerAcceptsTrailingSlashAudience(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, nil, func(claims *hooks.Claims) {
		claims.Audience = server.URL + "/"
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHTTPHandlerHonorsForwardedProto(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
	t.Cleanup(server.Close)
	httpsAudience := "https" + strings.TrimPrefix(server.URL, "http")
	response := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, nil, func(claims *hooks.Claims) {
		claims.Audience = httpsAudience
	}, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https, http")
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHTTPHandlerHonorsForwardedHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, nil, func(claims *hooks.Claims) {
		claims.Audience = "https://hooks.example.com"
	}, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Host", "hooks.example.com, internal-lb")
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func postEvent(t *testing.T, target string, eventType hooks.EventType, data any, updateRequest func(*hooks.Request), updateClaims func(*hooks.Claims), updateHTTPRequest ...func(*http.Request)) *http.Response {
	t.Helper()

	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	request := hooks.Request{
		Type: eventType,
		Meta: hooks.Meta{
			DispatchID:    uuid.New(),
			SchemaVersion: hooks.SchemaVersion,
			ChatRef: hooks.ChatRef{
				ChatID:  uuid.New(),
				OwnerID: uuid.New(),
			},
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
	token, err := hooks.SignClaims(testSecret, claims)
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

func validClaims(t *testing.T, audience string, eventType hooks.EventType, request *hooks.Request) hooks.Claims {
	t.Helper()

	now := time.Now()
	claims := hooks.Claims{
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

func TestHTTPHandlerRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(hooks.NewHTTPHandler(testSecret, hooks.Hooks{}))
	t.Cleanup(server.Close)

	// A correctly signed body over the limit must be rejected by size
	// before it is hashed or decoded.
	huge, err := json.Marshal(strings.Repeat("a", int(hooks.MaxRequestBodyBytes)))
	require.NoError(t, err)
	response := postEvent(t, server.URL, hooks.EventStop, hooks.StopData{}, func(request *hooks.Request) {
		request.Data = huge
	}, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}
