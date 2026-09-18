package codersdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestAIProviderSettings_UpstreamHeadersMarshal(t *testing.T) {
	t.Parallel()

	t.Run("HeadersEmitDiscriminator", func(t *testing.T) {
		t.Parallel()
		got, err := json.Marshal(codersdk.AIProviderSettings{
			UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
				Headers: map[string]string{"x-opencode-session": "session-123"},
			},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{
			"_type": "upstream-headers",
			"_version": 1,
			"headers": {"x-opencode-session": "session-123"}
		}`, string(got))
	})

	t.Run("EmptyHeadersEmitNull", func(t *testing.T) {
		t.Parallel()
		for _, s := range []codersdk.AIProviderSettings{
			{},
			{UpstreamHeaders: nil},
			{UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{}},
			{UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{Headers: map[string]string{}}},
		} {
			got, err := json.Marshal(s)
			require.NoError(t, err)
			require.JSONEq(t, `null`, string(got))
		}
	})

	t.Run("BedrockAndHeadersRefusesMarshal", func(t *testing.T) {
		t.Parallel()
		// The wire form carries a single discriminator, so a value holding
		// both variants must fail loudly rather than drop one silently.
		// Request validation rejects this shape before it can be stored.
		_, err := json.Marshal(codersdk.AIProviderSettings{
			Bedrock:         &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
			UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{Headers: map[string]string{"X-A": "b"}},
		})
		require.ErrorContains(t, err, "cannot combine bedrock and upstream-headers")
	})
}

func TestAIProviderSettings_UpstreamHeadersUnmarshal(t *testing.T) {
	t.Parallel()

	t.Run("SupportedVersion", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		require.NoError(t, json.Unmarshal([]byte(`{
			"_type": "upstream-headers",
			"_version": 1,
			"headers": {"x-opencode-session": "session-123"}
		}`), &s))
		require.Nil(t, s.Bedrock)
		require.NotNil(t, s.UpstreamHeaders)
		require.Equal(t, map[string]string{"x-opencode-session": "session-123"}, s.UpstreamHeaders.Headers)
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		err := json.Unmarshal([]byte(`{"_type":"upstream-headers","_version":99}`), &s)
		require.ErrorContains(t, err, `unsupported "upstream-headers" settings version 99`)
		require.ErrorContains(t, err, "expected 1")
	})

	t.Run("BedrockRowsStillDecode", func(t *testing.T) {
		t.Parallel()
		// Pre-existing rows without headers must keep decoding.
		var s codersdk.AIProviderSettings
		require.NoError(t, json.Unmarshal([]byte(`{
			"_type": "bedrock",
			"_version": 1,
			"region": "us-east-1"
		}`), &s))
		require.NotNil(t, s.Bedrock)
		require.Nil(t, s.UpstreamHeaders)
		require.False(t, s.IsZero())
	})
}

func TestAIProviderSettings_UpstreamHeadersRoundtrip(t *testing.T) {
	t.Parallel()
	orig := codersdk.AIProviderSettings{
		UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
			Headers: map[string]string{
				"x-opencode-session": "{{chat_id}}",
				"X-Custom":           "literal",
			},
		},
	}
	encoded, err := json.Marshal(orig)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(encoded), `"_type":"upstream-headers"`))

	var got codersdk.AIProviderSettings
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.Equal(t, orig, got)
}

func TestAIProviderRequest_ValidateUpstreamHeaders(t *testing.T) {
	t.Parallel()

	valid := func() codersdk.CreateAIProviderRequest {
		return codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAICompat,
			Name:    "zen",
			BaseURL: "https://opencode.ai/zen/go/v1",
			APIKeys: []string{"key"},
			Settings: codersdk.AIProviderSettings{
				UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
					Headers: map[string]string{"x-opencode-session": "{{chat_id}}"},
				},
			},
		}
	}

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, valid().Validate())
	})

	t.Run("InvalidHeaderName", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"not a header": "v"}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "valid HTTP token")
	})

	t.Run("DeniedHeaders", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Authorization", "X-Api-Key", "Host", "Content-Length"} {
			req := valid()
			req.Settings.UpstreamHeaders.Headers = map[string]string{name: "v"}
			errs := req.Validate()
			require.Len(t, errs, 1, "header %q", name)
			require.Contains(t, errs[0].Detail, "managed by the gateway")
		}
	})

	t.Run("EmptyValue", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"X-A": ""}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "must not be empty")
	})

	t.Run("CRLFValue", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"X-A": "a\r\nB: c"}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "CR or LF")
	})

	t.Run("DuplicateCaseInsensitive", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"X-A": "1", "x-a": "2"}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "case-insensitive")
	})

	t.Run("TooManyHeaders", func(t *testing.T) {
		t.Parallel()
		req := valid()
		headers := make(map[string]string, codersdk.MaxAIProviderUpstreamHeaders+1)
		for i := 0; i <= codersdk.MaxAIProviderUpstreamHeaders; i++ {
			headers["X-H-"+string(rune('a'+i%26))+string(rune('0'+i/26))] = "v"
		}
		req.Settings.UpstreamHeaders.Headers = headers
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "at most")
	})

	t.Run("UnknownPlaceholder", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"X-A": "{{session}}"}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "only")
	})

	t.Run("UnterminatedPlaceholder", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Settings.UpstreamHeaders.Headers = map[string]string{"X-A": "{{chat_id"}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "unterminated")
	})

	t.Run("BedrockAndHeadersRejected", func(t *testing.T) {
		t.Parallel()
		req := valid()
		req.Type = codersdk.AIProviderTypeAnthropic
		req.Settings.Bedrock = &codersdk.AIProviderBedrockSettings{Region: "us-east-1"}
		errs := req.Validate()
		require.NotEmpty(t, errs)
		require.Contains(t, errs[0].Detail, "only one settings type")
	})

	t.Run("UpdateRejectsCombo", func(t *testing.T) {
		t.Parallel()
		req := codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock:         &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
				UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{Headers: map[string]string{"X-A": "b"}},
			},
		}
		errs := req.Validate()
		require.NotEmpty(t, errs)
		require.Contains(t, errs[0].Detail, "only one settings type")
	})

	t.Run("UpdateValidatesHeaders", func(t *testing.T) {
		t.Parallel()
		req := codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				UpstreamHeaders: &codersdk.AIProviderUpstreamHeadersSettings{
					Headers: map[string]string{"Authorization": "v"},
				},
			},
		}
		errs := req.Validate()
		require.Len(t, errs, 1)
		require.Contains(t, errs[0].Detail, "managed by the gateway")
	})
}
