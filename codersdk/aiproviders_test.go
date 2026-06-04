package codersdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestAIProviderSettings_Marshal(t *testing.T) {
	t.Parallel()

	t.Run("EmptyEmitsNull", func(t *testing.T) {
		t.Parallel()
		got, err := json.Marshal(codersdk.AIProviderSettings{})
		require.NoError(t, err)
		require.JSONEq(t, `null`, string(got))
	})

	t.Run("BedrockEmitsDiscriminator", func(t *testing.T) {
		t.Parallel()
		got, err := json.Marshal(codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				Model:           "anthropic.claude-3-5-sonnet",
				SmallFastModel:  "anthropic.claude-3-5-haiku",
				AccessKey:       ptr.Ref("AKIA-test"), //nolint:gosec // fixture
				AccessKeySecret: ptr.Ref("secret"),
			},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{
			"_type": "bedrock",
			"_version": 1,
			"region": "us-east-1",
			"model": "anthropic.claude-3-5-sonnet",
			"small_fast_model": "anthropic.claude-3-5-haiku",
			"access_key": "AKIA-test",
			"access_key_secret": "secret"
		}`, string(got))
	})

	t.Run("BedrockOmitsEmptyFields", func(t *testing.T) {
		t.Parallel()
		got, err := json.Marshal(codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{
			"_type": "bedrock",
			"_version": 1,
			"region": "us-east-1"
		}`, string(got))
	})
}

func TestAIProviderSettings_Unmarshal(t *testing.T) {
	t.Parallel()

	t.Run("EmptyInputZeroes", func(t *testing.T) {
		t.Parallel()
		// encoding/json never invokes UnmarshalJSON with an empty
		// payload, but the method must still tolerate it for callers
		// (e.g. row decoders) that hand it raw column bytes.
		var s codersdk.AIProviderSettings
		require.NoError(t, s.UnmarshalJSON(nil))
		require.True(t, s.IsZero())
		require.NoError(t, s.UnmarshalJSON([]byte("")))
		require.True(t, s.IsZero())
	})

	t.Run("NullZeroes", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		require.NoError(t, json.Unmarshal([]byte(`null`), &s))
		require.True(t, s.IsZero())
	})

	t.Run("EmptyObjectZeroes", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		require.NoError(t, json.Unmarshal([]byte(`{}`), &s))
		require.True(t, s.IsZero())
	})

	t.Run("BedrockSupportedVersion", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		require.NoError(t, json.Unmarshal([]byte(`{
			"_type":    "bedrock",
			"_version": 1,
			"region":   "us-east-1",
			"model":    "anthropic.claude-3-5-sonnet"
		}`), &s))
		require.NotNil(t, s.Bedrock)
		require.Equal(t, "us-east-1", s.Bedrock.Region)
		require.Equal(t, "anthropic.claude-3-5-sonnet", s.Bedrock.Model)
	})

	t.Run("MissingTypeDiscriminator", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		err := json.Unmarshal([]byte(`{"_version":1,"region":"us-east-1"}`), &s)
		require.ErrorContains(t, err, "missing _type discriminator")
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		err := json.Unmarshal([]byte(`{"_type":"bedrock","_version":99}`), &s)
		require.ErrorContains(t, err, `unsupported "bedrock" settings version 99`)
		require.ErrorContains(t, err, "expected 1")
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		var s codersdk.AIProviderSettings
		err := json.Unmarshal([]byte(`{"_type":"copilot","_version":1}`), &s)
		require.ErrorContains(t, err, `unknown settings type "copilot"`)
	})

	t.Run("MalformedHeader", func(t *testing.T) {
		t.Parallel()
		// _type must be a string; passing a number triggers the
		// header decode path before any discriminator routing.
		var s codersdk.AIProviderSettings
		err := json.Unmarshal([]byte(`{"_type": 1}`), &s)
		require.ErrorContains(t, err, "decode settings header")
		require.ErrorContains(t, err, "_type")
	})

	t.Run("ResetsBetweenCalls", func(t *testing.T) {
		t.Parallel()
		// A non-zero value passed to Unmarshal should be reset when
		// the payload decodes to null, so callers can reuse the
		// variable without leaking stale state.
		s := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
		}
		require.NoError(t, json.Unmarshal([]byte(`null`), &s))
		require.True(t, s.IsZero())
	})
}

func TestAIProviderSettings_Roundtrip(t *testing.T) {
	t.Parallel()
	orig := codersdk.AIProviderSettings{
		Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:          "us-west-2",
			Model:           "anthropic.claude-sonnet-4-5",
			SmallFastModel:  "anthropic.claude-haiku-4-5",
			AccessKey:       ptr.Ref("AKIA-roundtrip"), //nolint:gosec // fixture
			AccessKeySecret: ptr.Ref("secret-roundtrip"),
		},
	}
	encoded, err := json.Marshal(orig)
	require.NoError(t, err)
	// Sanity: discriminator is part of the on-wire shape.
	require.True(t, strings.Contains(string(encoded), `"_type":"bedrock"`))

	var got codersdk.AIProviderSettings
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.Equal(t, orig, got)
}
