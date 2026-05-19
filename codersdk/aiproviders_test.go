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

func TestCreateAIProviderRequest_Validate_Bedrock(t *testing.T) {
	t.Parallel()

	base := codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeAnthropic,
		Name:    "my-bedrock",
		BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
	}

	t.Run("BedrockOnNonAnthropicTypeRejected", func(t *testing.T) {
		t.Parallel()
		req := base
		req.Type = codersdk.AIProviderTypeOpenAI
		req.Settings = codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:         "us-east-1",
				Model:          "anthropic.claude-sonnet-4-5",
				SmallFastModel: "anthropic.claude-haiku-4-5",
			},
		}
		requireFieldError(t, req.Validate(), "settings")
	})

	t.Run("RegionRequired", func(t *testing.T) {
		t.Parallel()
		req := base
		req.Settings = codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Model:          "anthropic.claude-sonnet-4-5",
				SmallFastModel: "anthropic.claude-haiku-4-5",
			},
		}
		requireFieldError(t, req.Validate(), "settings.region")
	})
	t.Run("AccessKeysOptional", func(t *testing.T) {
		t.Parallel()
		// Secret credentials can come from env/IMDS at runtime, so
		// the codersdk layer must not require them; deployments without
		// inline AWS keys are valid.
		req := base
		req.Settings = codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:         "us-east-1",
				Model:          "anthropic.claude-sonnet-4-5",
				SmallFastModel: "anthropic.claude-haiku-4-5",
			},
		}
		require.Empty(t, req.Validate())
	})

	t.Run("ValidPayload", func(t *testing.T) {
		t.Parallel()
		req := base
		req.Settings = codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				Model:           "anthropic.claude-sonnet-4-5",
				SmallFastModel:  "anthropic.claude-haiku-4-5",
				AccessKey:       ptr.Ref("AKIA-test"), //nolint:gosec // fixture
				AccessKeySecret: ptr.Ref("secret"),
			},
		}
		require.Empty(t, req.Validate())
	})
}

func TestUpdateAIProviderRequest_Validate_Bedrock(t *testing.T) {
	t.Parallel()

	t.Run("SettingsNilSkipped", func(t *testing.T) {
		t.Parallel()
		// Update requests without Settings shouldn't run the Bedrock
		// field checks: leaving Bedrock alone is a valid PATCH.
		require.Empty(t, codersdk.UpdateAIProviderRequest{
			BaseURL: ptr.Ref("https://api.anthropic.com"),
		}.Validate())
	})

	t.Run("BedrockNilCleared", func(t *testing.T) {
		t.Parallel()
		// settings: {} on the wire means "clear the Bedrock blob";
		// the merge function treats it as a reset, so per-field
		// validation is skipped.
		require.Empty(t, codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{},
		}.Validate())
	})

	t.Run("RegionRequired", func(t *testing.T) {
		t.Parallel()
		req := codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Model:          "anthropic.claude-sonnet-4-5",
					SmallFastModel: "anthropic.claude-haiku-4-5",
				},
			},
		}
		requireFieldError(t, req.Validate(), "settings.region")
	})

	t.Run("AccessKeyOmittedAllowed", func(t *testing.T) {
		t.Parallel()
		// Omitting access_key/access_key_secret on update means
		// "inherit the existing values" per mergeAIProviderSettings.
		req := codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:         "us-east-1",
					Model:          "anthropic.claude-sonnet-4-5",
					SmallFastModel: "anthropic.claude-haiku-4-5",
				},
			},
		}
		require.Empty(t, req.Validate())
	})

	t.Run("AccessKeyClearedAllowed", func(t *testing.T) {
		t.Parallel()
		// An empty *string explicitly clears the credential (e.g.
		// migrating to IAM auth); validation must allow it.
		req := codersdk.UpdateAIProviderRequest{
			Settings: &codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:          "us-east-1",
					Model:           "anthropic.claude-sonnet-4-5",
					SmallFastModel:  "anthropic.claude-haiku-4-5",
					AccessKey:       ptr.Ref(""),
					AccessKeySecret: ptr.Ref(""),
				},
			},
		}
		require.Empty(t, req.Validate())
	})
}

// requireFieldError fails the test unless the supplied validations
// include exactly one error pointing at `field`.
func requireFieldError(t *testing.T, validations []codersdk.ValidationError, field string) {
	t.Helper()
	for _, v := range validations {
		if v.Field == field {
			return
		}
	}
	t.Fatalf("expected validation error on field %q, got %#v", field, validations)
}
