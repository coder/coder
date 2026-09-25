package headers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/recorder"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

func TestActorHeaders(t *testing.T) {
	t.Parallel()

	require.Equal(t, "X-AI-Bridge-Actor-ID", headers.ActorIDHeader())
	require.Equal(t, "X-AI-Bridge-Actor-Metadata-Username", headers.ActorMetadataHeader("Username"))

	require.True(t, headers.IsActorHeader(headers.ActorIDHeader()))
	require.True(t, headers.IsActorHeader("x-ai-bridge-actor-metadata-name"))
	require.False(t, headers.IsActorHeader("X-AI-Bridge-Request-ID"))
}

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty",
			input:    "",
			expected: "",
		},
		{
			name:     "Whitespace",
			input:    " ",
			expected: "",
		},
		{
			name:     "InvalidFormat",
			input:    "some-token",
			expected: "",
		},
		{
			name:     "BearerOnly",
			input:    "Bearer",
			expected: "",
		},
		{
			name:     "Valid",
			input:    "Bearer my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "BearerMixedCase",
			input:    "BeArEr my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "LeadingWhitespace",
			input:    "  Bearer my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "TrailingWhitespace",
			input:    "Bearer my-secret-token  ",
			expected: "my-secret-token",
		},
		{
			name:     "TooManyParts",
			input:    "Bearer token extra",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := headers.ExtractBearerToken(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		connection string
		upgrade    string
		want       bool
	}{
		{name: "websocket upgrade", method: http.MethodGet, connection: "keep-alive, Upgrade", upgrade: "WebSocket", want: true},
		{name: "non-GET request", method: http.MethodPost, connection: "Upgrade", upgrade: "websocket", want: false},
		{name: "missing connection upgrade", method: http.MethodGet, connection: "keep-alive", upgrade: "websocket", want: false},
		{name: "different upgrade protocol", method: http.MethodGet, connection: "Upgrade", upgrade: "h2c", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "/", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			req.Header.Set("Upgrade", tc.upgrade)

			assert.Equal(t, tc.want, headers.IsWebSocketUpgrade(req))
		})
	}
}

func TestExtractAgentFirewallHeaders(t *testing.T) {
	t.Parallel()

	const validSessionID = "e5f6a7b8-1234-5678-9abc-def012345678"

	ptr := func(s string) *string { return &s }

	cases := []struct {
		name string
		// sessionID and seqNumber set the corresponding headers when
		// non-nil. A nil value leaves the header unset.
		sessionID *string
		seqNumber *string

		wantErr     bool
		errContains string
		wantSession *string
		wantSeq     *int32
	}{
		{
			name:        "both headers present",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("42"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(42),
		},
		{
			name: "no headers present",
		},
		{
			name:        "only session ID returns error",
			sessionID:   ptr(validSessionID),
			wantErr:     true,
			errContains: "without sequence number",
		},
		{
			name:        "only sequence number returns error",
			seqNumber:   ptr("7"),
			wantErr:     true,
			errContains: "without session ID",
		},
		{
			name:        "sequence number zero",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("0"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(0),
		},
		{
			name:        "invalid session ID returns error",
			sessionID:   ptr("not-a-uuid"),
			seqNumber:   ptr("42"),
			wantErr:     true,
			errContains: "invalid agent firewall session ID",
		},
		{
			name:        "invalid sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("not-a-number"),
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
		{
			name:        "negative sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("-1"),
			wantErr:     true,
			errContains: "must be non-negative",
		},
		{
			name:        "sequence number exceeding int32 range returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("2147483648"), // max int32 + 1
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			require.NoError(t, err)
			if tc.sessionID != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, *tc.sessionID)
			}
			if tc.seqNumber != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, *tc.seqNumber)
			}

			sessionID, seqNumber, extractErr := headers.ExtractAgentFirewallHeaders(req)

			if tc.wantErr {
				require.Error(t, extractErr)
				assert.Contains(t, extractErr.Error(), tc.errContains)
				assert.Nil(t, sessionID)
				assert.Nil(t, seqNumber)
				return
			}

			require.NoError(t, extractErr)
			if tc.wantSession == nil {
				assert.Nil(t, sessionID)
			} else {
				require.NotNil(t, sessionID)
				assert.Equal(t, *tc.wantSession, *sessionID)
			}
			if tc.wantSeq == nil {
				assert.Nil(t, seqNumber)
			} else {
				require.NotNil(t, seqNumber)
				assert.Equal(t, *tc.wantSeq, *seqNumber)
			}
		})
	}
}

func TestPrepareClientHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty header", func(t *testing.T) {
		t.Parallel()

		result := headers.PrepareClientHeaders(nil)
		require.Empty(t, result)
	})

	t.Run("hop-by-hop headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection":        {"keep-alive"},
			"Keep-Alive":        {"timeout=5"},
			"Transfer-Encoding": {"chunked"},
			"Upgrade":           {"websocket"},
			"X-Custom":          {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Connection"))
		assert.Empty(t, result.Get("Keep-Alive"))
		assert.Empty(t, result.Get("Transfer-Encoding"))
		assert.Empty(t, result.Get("Upgrade"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("non-forwarded headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Host":            {"example.com"},
			"Accept-Encoding": {"gzip"},
			"Content-Length":  {"42"},
			"X-Custom":        {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Host"))
		assert.Empty(t, result.Get("Accept-Encoding"))
		assert.Empty(t, result.Get("Content-Length"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("auth headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Authorization": {"Bearer coder-session-token"},
			"X-Api-Key":     {"sk-client-key"},
			"X-Custom":      {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Authorization"))
		assert.Empty(t, result.Get("X-Api-Key"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("proxy headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Forwarded-For":   {"203.0.113.50"},
			"X-Forwarded-Host":  {"app.example.com"},
			"X-Forwarded-Proto": {"https"},
			"X-Forwarded-Port":  {"443"},
			"Forwarded":         {"for=203.0.113.50;proto=https"},
			"X-Custom":          {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("X-Forwarded-For"))
		assert.Empty(t, result.Get("X-Forwarded-Host"))
		assert.Empty(t, result.Get("X-Forwarded-Proto"))
		assert.Empty(t, result.Get("X-Forwarded-Port"))
		assert.Empty(t, result.Get("Forwarded"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("multi-value headers are preserved", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Custom": {"value-1", "value-2"},
		}

		result := headers.PrepareClientHeaders(input)

		require.Equal(t, []string{"value-1", "value-2"}, result["X-Custom"])
	})

	t.Run("input is not mutated", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection": {"keep-alive"},
			"X-Custom":   {"preserved"},
		}
		originalCopy := input.Clone()

		_ = headers.PrepareClientHeaders(input)

		require.Equal(t, originalCopy, input)
	})

	t.Run("agent firewall headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Coder-Agent-Firewall-Session-Id":      {"e5f6a7b8-1234-5678-9abc-def012345678"},
			"X-Coder-Agent-Firewall-Sequence-Number": {"42"},
			"X-Custom":                               {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Session-Id"))
		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Sequence-Number"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})
}

func TestBuildUpstreamHeaders(t *testing.T) {
	t.Parallel()

	t.Run("preserves auth from SDK", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-provider-key"},
		}
		clientHeaders := http.Header{
			"Authorization": {"Bearer coder-session-token"},
			"User-Agent":    {"claude-code/1.0"},
		}

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil, nil)

		assert.Equal(t, "Bearer sk-provider-key", result.Get("Authorization"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("preserves X-Api-Key from SDK and strips client Authorization", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"X-Api-Key": {"sk-ant-provider-key"},
		}
		clientHeaders := http.Header{
			"X-Api-Key":      {"sk-ant-client-key"},
			"Authorization":  {"Bearer coder-session-token"},
			"Anthropic-Beta": {"prompt-caching-2024-07-31"},
		}

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "X-Api-Key", false, nil, nil)

		assert.Equal(t, "sk-ant-provider-key", result.Get("X-Api-Key"))
		assert.Empty(t, result.Get("Authorization"))
		assert.Equal(t, "prompt-caching-2024-07-31", result.Get("Anthropic-Beta"))
	})

	t.Run("strips hop-by-hop and transport headers", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-key"},
		}
		clientHeaders := http.Header{
			"Connection":        {"keep-alive"},
			"Host":              {"bridge.example.com"},
			"Content-Length":    {"99"},
			"Accept-Encoding":   {"gzip"},
			"Transfer-Encoding": {"chunked"},
			"User-Agent":        {"claude-code/1.0"},
		}

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil, nil)

		assert.Empty(t, result.Get("Connection"))
		assert.Empty(t, result.Get("Host"))
		assert.Empty(t, result.Get("Content-Length"))
		assert.Empty(t, result.Get("Accept-Encoding"))
		assert.Empty(t, result.Get("Transfer-Encoding"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("empty auth header in SDK is not injected", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{}
		clientHeaders := http.Header{
			"User-Agent": {"claude-code/1.0"},
		}

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil, nil)

		assert.Empty(t, result.Get("Authorization"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("does not mutate inputs", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-key"},
		}
		clientHeaders := http.Header{
			"Authorization": {"Bearer coder-token"},
			"Connection":    {"keep-alive"},
		}
		sdkCopy := sdkHeader.Clone()
		clientCopy := clientHeaders.Clone()

		_ = headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil, nil)

		require.Equal(t, sdkCopy, sdkHeader)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("actor forwarding off ignores SDK actor headers and preserves client values", func(t *testing.T) {
		t.Parallel()

		sdkHeaders := http.Header{}
		sdkHeaders.Set("Authorization", "Bearer provider-key")
		sdkHeaders.Set(headers.ActorIDHeader(), "sdk-id")
		sdkHeaders.Set(headers.ActorMetadataHeader("Username"), "sdk-name")
		clientHeaders := http.Header{}
		clientHeaders.Set("Authorization", "Bearer client-key")
		clientHeaders.Set(headers.ActorIDHeader(), "client-id")
		clientHeaders.Set(headers.ActorMetadataHeader("Username"), "client-name")
		sdkCopy, clientCopy := sdkHeaders.Clone(), clientHeaders.Clone()

		result := headers.BuildUpstreamHeaders(sdkHeaders, clientHeaders, "Authorization", false, nil, nil)

		require.Equal(t, "client-id", result.Get(headers.ActorIDHeader()))
		require.Equal(t, "client-name", result.Get(headers.ActorMetadataHeader("Username")))
		require.Equal(t, "Bearer provider-key", result.Get("Authorization"))
		require.Equal(t, sdkCopy, sdkHeaders)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("mapped actor destinations replace defaults and stale values", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name  string
			actor *context.Actor
			want  map[string]string
		}{
			{
				name:  "configured actor",
				actor: &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}},
				want:  map[string]string{"X-Downstream-User-Id": "user-123", "X-Downstream-Username": "alice"},
			},
			{name: "nil actor", want: map[string]string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				sdkHeaders := http.Header{}
				sdkHeaders.Set("X-Downstream-User-Id", "sdk-id")
				sdkHeaders.Set("X-Downstream-Username", "sdk-name")
				clientHeaders := http.Header{}
				clientHeaders.Set("X-Downstream-User-Id", "client-id")
				clientHeaders.Set("X-Downstream-Username", "client-name")
				sdkCopy, clientCopy := sdkHeaders.Clone(), clientHeaders.Clone()

				result := headers.BuildUpstreamHeaders(sdkHeaders, clientHeaders, "Authorization", true, map[string]string{
					"id":       "X-Downstream-User-Id",
					"username": "X-Downstream-Username",
				}, tc.actor)

				if tc.actor == nil {
					require.NotContains(t, result, "X-Downstream-User-Id")
					require.NotContains(t, result, "X-Downstream-Username")
				} else {
					require.Equal(t, tc.want, map[string]string{
						"X-Downstream-User-Id":  result.Get("X-Downstream-User-Id"),
						"X-Downstream-Username": result.Get("X-Downstream-Username"),
					})
				}
				require.Equal(t, sdkCopy, sdkHeaders)
				require.Equal(t, clientCopy, clientHeaders)
			})
		}
	})

	t.Run("actor forwarding on strips standard destinations with custom mapping", func(t *testing.T) {
		t.Parallel()

		sdkHeaders := http.Header{}
		sdkHeaders.Set(headers.ActorIDHeader(), "sdk-id")
		sdkHeaders.Set(headers.ActorMetadataHeader("Username"), "sdk-name")
		sdkHeaders.Set("Authorization", "Bearer provider-key")
		clientHeaders := http.Header{}
		clientHeaders.Set(headers.ActorIDHeader(), "client-id")
		clientHeaders.Set(headers.ActorMetadataHeader("Username"), "client-name")
		clientHeaders.Set("X-Unrelated", "preserved")
		sdkCopy, clientCopy := sdkHeaders.Clone(), clientHeaders.Clone()

		result := headers.BuildUpstreamHeaders(sdkHeaders, clientHeaders, "Authorization", true, map[string]string{"id": "X-Downstream-User-Id"}, &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}})

		require.Equal(t, "user-123", result.Get("X-Downstream-User-Id"))
		require.NotContains(t, result, http.CanonicalHeaderKey(headers.ActorIDHeader()))
		require.NotContains(t, result, http.CanonicalHeaderKey(headers.ActorMetadataHeader("Username")))
		require.Equal(t, "Bearer provider-key", result.Get("Authorization"))
		require.Equal(t, "preserved", result.Get("X-Unrelated"))
		require.Equal(t, sdkCopy, sdkHeaders)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("authenticated actor overrides client and SDK values", func(t *testing.T) {
		t.Parallel()

		sdkHeaders := http.Header{
			"Authorization":                       {"Bearer provider-key"},
			"X-Ai-Bridge-Actor-Id":                {"sdk-id"},
			"X-Ai-Bridge-Actor-Metadata-Username": {"sdk-name"},
		}
		clientHeaders := http.Header{
			"X-Ai-Bridge-Actor-Id":                {"client-id"},
			"X-Ai-Bridge-Actor-Metadata-Username": {"client-name"},
		}
		sdkCopy, clientCopy := sdkHeaders.Clone(), clientHeaders.Clone()
		actor := &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}}

		result := headers.BuildUpstreamHeaders(sdkHeaders, clientHeaders, "Authorization", true, map[string]string{
			"id":       headers.ActorIDHeader(),
			"username": headers.ActorMetadataHeader("Username"),
		}, actor)

		require.Equal(t, []string{"user-123"}, result.Values(headers.ActorIDHeader()))
		require.Equal(t, []string{"alice"}, result.Values(headers.ActorMetadataHeader("Username")))
		require.Equal(t, "Bearer provider-key", result.Get("Authorization"))
		require.Equal(t, sdkCopy, sdkHeaders)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("actor forwarding with nil client headers", func(t *testing.T) {
		t.Parallel()

		actor := &context.Actor{ID: "user-123"}
		for _, tc := range []struct {
			name  string
			send  bool
			actor *context.Actor
			want  string
		}{
			{name: "enabled", send: true, actor: actor, want: actor.ID},
			{name: "disabled", actor: actor},
			{name: "nil actor", send: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				result := headers.BuildUpstreamHeaders(nil, nil, "Authorization", tc.send, map[string]string{"id": headers.ActorIDHeader()}, tc.actor)
				require.Equal(t, tc.want, result.Get(headers.ActorIDHeader()))
			})
		}
	})
}

func int32Ptr(n int32) *int32 { return &n }
