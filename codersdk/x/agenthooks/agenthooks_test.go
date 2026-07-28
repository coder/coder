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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

// testAudience is the URL the handler is configured to accept, kept
// independent of the test server address it is reached on.
const testAudience = "https://hooks.example.com"

func TestSignClaimsVerify(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", agenthooks.EventPreToolUse, nil)
	token, err := agenthooks.SignClaims(testSecret, claims)
	require.NoError(t, err)

	got, err := agenthooks.Verify("Bearer "+token, testSecret)
	require.NoError(t, err)
	require.Equal(t, claims, got)
}

func TestShortSecretRejected(t *testing.T) {
	t.Parallel()

	claims := validClaims(t, "https://hooks.example.com/coder", agenthooks.EventPreToolUse, nil)
	shortSecret := testSecret[:agenthooks.MinSecretLen-1]

	_, err := agenthooks.SignClaims(shortSecret, claims)
	require.ErrorContains(t, err, "secret must be at least")

	token, err := agenthooks.SignClaims(testSecret, claims)
	require.NoError(t, err)
	_, err = agenthooks.Verify("Bearer "+token, shortSecret)
	require.ErrorContains(t, err, "secret must be at least")
	_, err = agenthooks.Verify("Bearer "+token, nil)
	require.ErrorContains(t, err, "secret must be at least")
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
	var algErr *jose.ErrUnexpectedSignatureAlgorithm
	require.ErrorAs(t, err, &algErr)
	require.Equal(t, jose.HS512, algErr.Got)
}

func TestVerifyTimeBounds(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name    string
		update  func(*agenthooks.Claims)
		wantErr string
	}{
		{
			name: "expired",
			update: func(claims *agenthooks.Claims) {
				claims.IssuedAt = now.Add(-2 * time.Minute).Unix()
				claims.NotBefore = now.Add(-2 * time.Minute).Unix()
				claims.Expires = now.Add(-time.Minute).Unix()
			},
			wantErr: "token has expired",
		},
		{
			name: "not before",
			update: func(claims *agenthooks.Claims) {
				claims.NotBefore = now.Add(time.Minute).Unix()
				claims.Expires = now.Add(2 * time.Minute).Unix()
			},
			wantErr: "token is not valid yet",
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
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestHTTPHandlerRoutesEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   agenthooks.EventType
		data    any
		install func(*testing.T, *agenthooks.Hooks)
	}{
		{
			name:  "session start",
			event: agenthooks.EventSessionStart,
			data:  agenthooks.SessionStartData{Source: "startup"},
			install: func(t *testing.T, h *agenthooks.Hooks) {
				h.SessionStart = func(_ context.Context, _ agenthooks.Meta, data agenthooks.SessionStartData) (agenthooks.Response, error) {
					assert.Equal(t, "startup", data.Source)
					return agenthooks.Response{UserMessage: "session start"}, nil
				}
			},
		},
		{
			name:  "user prompt submit",
			event: agenthooks.EventUserPromptSubmit,
			data:  agenthooks.UserPromptSubmitData{Prompt: "hello"},
			install: func(t *testing.T, h *agenthooks.Hooks) {
				h.UserPromptSubmit = func(_ context.Context, _ agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
					assert.Equal(t, "hello", data.Prompt)
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
			install: func(t *testing.T, h *agenthooks.Hooks) {
				h.PreToolUse = func(_ context.Context, _ agenthooks.Meta, data agenthooks.PreToolUseData) (agenthooks.Response, error) {
					assert.Equal(t, "execute", data.ToolName)
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
			install: func(t *testing.T, h *agenthooks.Hooks) {
				h.PostToolUse = func(_ context.Context, _ agenthooks.Meta, data agenthooks.PostToolUseData) (agenthooks.Response, error) {
					assert.Equal(t, "execute", data.ToolName)
					return agenthooks.Response{UserMessage: "post tool use"}, nil
				}
			},
		},
		{
			name:  "pre compact",
			event: agenthooks.EventPreCompact,
			data:  agenthooks.PreCompactData{},
			install: func(_ *testing.T, h *agenthooks.Hooks) {
				h.PreCompact = func(context.Context, agenthooks.Meta, agenthooks.PreCompactData) (agenthooks.Response, error) {
					return agenthooks.Response{UserMessage: "pre compact"}, nil
				}
			},
		},
		{
			name:  "post compact",
			event: agenthooks.EventPostCompact,
			data:  agenthooks.PostCompactData{},
			install: func(_ *testing.T, h *agenthooks.Hooks) {
				h.PostCompact = func(context.Context, agenthooks.Meta, agenthooks.PostCompactData) (agenthooks.Response, error) {
					return agenthooks.Response{UserMessage: "post compact"}, nil
				}
			},
		},
		{
			name:  "stop",
			event: agenthooks.EventStop,
			data:  agenthooks.StopData{},
			install: func(_ *testing.T, h *agenthooks.Hooks) {
				h.Stop = func(context.Context, agenthooks.Meta, agenthooks.StopData) (agenthooks.Response, error) {
					return agenthooks.Response{UserMessage: "stop"}, nil
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var h agenthooks.Hooks
			test.install(t, &h)
			server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, h))
			t.Cleanup(server.Close)

			response := postEvent(t, server.URL, test.event, test.data, nil, nil)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var got agenthooks.Response
			require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
			require.Equal(t, test.name, got.UserMessage)
		})
	}
}

func TestHTTPHandlerUnencodableResponseFailsClosed(t *testing.T) {
	t.Parallel()

	// An empty 200 reads as allow, so a response that cannot be marshaled
	// must not reach the dispatcher as one.
	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{
		Stop: func(context.Context, agenthooks.Meta, agenthooks.StopData) (agenthooks.Response, error) {
			return agenthooks.Response{
				Permission: &agenthooks.Permission{
					Decision:      agenthooks.PermissionDeny,
					InputOverride: json.RawMessage(`{invalid`),
				},
			}, nil
		},
	}))
	t.Cleanup(server.Close)

	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
}

func TestHTTPHandlerNoOpHookDoesNotDecodeData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{}))
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

			server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{}))
			t.Cleanup(server.Close)
			response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, test.updateRequest, test.updateClaims)
			defer response.Body.Close()
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
		})
	}
}

func TestHTTPHandlerExpectedIssuer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(
		testSecret,
		testAudience,
		agenthooks.Hooks{},
		agenthooks.WithExpectedIssuer("deployment-a"),
	))
	t.Cleanup(server.Close)

	matching := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Issuer = "deployment-a"
	})
	defer matching.Body.Close()
	require.Equal(t, http.StatusOK, matching.StatusCode)

	mismatched := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Issuer = "deployment-b"
	})
	defer mismatched.Body.Close()
	require.Equal(t, http.StatusUnauthorized, mismatched.StatusCode)
}

func TestHTTPHandlerAcceptsTrailingSlashAudience(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, func(claims *agenthooks.Claims) {
		claims.Audience = testAudience + "/"
	})
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHTTPHandlerRejectsRequestDerivedAudience(t *testing.T) {
	t.Parallel()

	// Every request-controlled source of an audience names an attacker host, so
	// a token minted for another listener sharing this secret would pass an
	// audience check that reads any of them.
	const spoofed = "https://hooks.attacker.example"
	handler := agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{})
	body, token := signedEvent(t, spoofed, agenthooks.EventStop, agenthooks.StopData{}, nil, nil)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, spoofed, bytes.NewReader(body))
	require.NoError(t, err)
	request.Host = "hooks.attacker.example"
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "hooks.attacker.example")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestHTTPHandlerWithoutAudienceRejectsEveryRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, "", agenthooks.Hooks{}))
	t.Cleanup(server.Close)
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, nil, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
}

func postEvent(t *testing.T, target string, eventType agenthooks.EventType, data any, updateRequest func(*agenthooks.Request), updateClaims func(*agenthooks.Claims)) *http.Response {
	t.Helper()

	body, token := signedEvent(t, testAudience, eventType, data, updateRequest, updateClaims)
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(httpRequest)
	require.NoError(t, err)
	return response
}

func signedEvent(t *testing.T, audience string, eventType agenthooks.EventType, data any, updateRequest func(*agenthooks.Request), updateClaims func(*agenthooks.Claims)) ([]byte, string) {
	t.Helper()

	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	request := agenthooks.Request{
		Type: eventType,
		Meta: agenthooks.Meta{
			DispatchID:    uuid.New(),
			SchemaVersion: agenthooks.SchemaVersion,
			ChatRef: agenthooks.ChatRef{
				ChatID:  uuid.New(),
				OwnerID: uuid.New(),
			},
		},
		Data: dataJSON,
	}
	claims := validClaims(t, audience, eventType, &request)
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
	return body, token
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

func TestHTTPHandlerRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(agenthooks.NewHTTPHandler(testSecret, testAudience, agenthooks.Hooks{}))
	t.Cleanup(server.Close)

	// A correctly signed body over the limit must be rejected by size
	// before it is hashed or decoded.
	huge, err := json.Marshal(strings.Repeat("a", int(agenthooks.MaxRequestBodyBytes)))
	require.NoError(t, err)
	response := postEvent(t, server.URL, agenthooks.EventStop, agenthooks.StopData{}, func(request *agenthooks.Request) {
		request.Data = huge
	}, nil)
	defer response.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}
