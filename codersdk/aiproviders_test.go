package codersdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
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
				AccessKey:       new("AKIA-test"), //nolint:gosec // fixture
				AccessKeySecret: new("secret"),
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
			AccessKey:       new("AKIA-roundtrip"), //nolint:gosec // fixture
			AccessKeySecret: new("secret-roundtrip"),
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

func TestAIProviderRequest_ValidateAPIKeys(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		keys            []string
		wantCreateField string
		wantUpdateField string
		wantDetail      string
	}{
		{name: "Omitted"},
		{name: "Empty", keys: []string{}},
		{name: "One", keys: []string{"key-1"}},
		{name: "Five", keys: []string{"key-1", "key-2", "key-3", "key-4", "key-5"}},
		{
			name:            "Six",
			keys:            []string{"key-1", "key-2", "key-3", "key-4", "key-5", "key-6"},
			wantCreateField: "api_keys",
			wantUpdateField: "api_keys",
			wantDetail:      "api_keys must contain at most 5 keys",
		},
		{
			name:            "Duplicate",
			keys:            []string{"key-1", "key-2", "key-1"},
			wantCreateField: "api_keys[2]",
			wantUpdateField: "api_keys[2].api_key",
			wantDetail:      "duplicate key already provided at api_keys[0]",
		},
		{
			name:            "DuplicateAfterDifferentKey",
			keys:            []string{"key-1", "key-2", "key-2"},
			wantCreateField: "api_keys[2]",
			wantUpdateField: "api_keys[2].api_key",
			wantDetail:      "duplicate key already provided at api_keys[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			create := codersdk.CreateAIProviderRequest{
				Type:    codersdk.AIProviderTypeOpenAI,
				Name:    "keys",
				BaseURL: "https://api.openai.com/v1",
				APIKeys: tc.keys,
			}
			createValidations := create.Validate()
			if tc.wantCreateField == "" {
				require.Empty(t, createValidations)
			} else {
				require.Len(t, createValidations, 1)
				require.Equal(t, tc.wantCreateField, createValidations[0].Field)
				require.Equal(t, tc.wantDetail, createValidations[0].Detail)
				for _, key := range tc.keys {
					require.NotContains(t, createValidations[0].Detail, key)
				}
			}

			muts := make([]codersdk.AIProviderKeyMutation, 0, len(tc.keys))
			for _, key := range tc.keys {
				muts = append(muts, codersdk.AIProviderKeyMutation{APIKey: new(key)})
			}
			update := codersdk.UpdateAIProviderRequest{APIKeys: &muts}
			updateValidations := update.Validate()
			if tc.wantUpdateField == "" {
				require.Empty(t, updateValidations)
			} else {
				require.Len(t, updateValidations, 1)
				require.Equal(t, tc.wantUpdateField, updateValidations[0].Field)
				require.Equal(t, tc.wantDetail, updateValidations[0].Detail)
				for _, key := range tc.keys {
					require.NotContains(t, updateValidations[0].Detail, key)
				}
			}
		})
	}
}

func TestValidateAIProviderKeyUniqueness(t *testing.T) {
	t.Parallel()

	seen := map[string]int{"key-1": 1}
	require.Empty(t, codersdk.ValidateAIProviderKeyUniqueness("key-2", "api_keys[2]", seen))
	require.Equal(t, map[string]int{"key-1": 1}, seen)

	require.Equal(t, []codersdk.ValidationError{{
		Field:  "api_keys[3]",
		Detail: "duplicate key already provided at api_keys[1]",
	}}, codersdk.ValidateAIProviderKeyUniqueness("key-1", "api_keys[3]", seen))
	require.Equal(t, map[string]int{"key-1": 1}, seen)
}

func TestAIProviderRequest_ValidateBedrockCredentials(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		key        *string
		secret     *string
		wantField  string
		wantDetail string
	}{
		{name: "Omitted"},
		{name: "Empty", key: new(""), secret: new("")},
		{name: "EmptyKey", key: new("")},
		{name: "EmptySecret", secret: new("")},
		{name: "Paired", key: new("key"), secret: new("longer-secret")},
		{
			name:       "MissingKey",
			secret:     new("secret"),
			wantField:  "settings.access_key",
			wantDetail: "access_key_secret is set, but access_key is missing or empty",
		},
		{
			name:       "MissingSecret",
			key:        new("key"),
			wantField:  "settings.access_key_secret",
			wantDetail: "access_key is set, but access_key_secret is missing or empty",
		},
		{
			name:       "ClearedKey",
			key:        new(""),
			secret:     new("secret"),
			wantField:  "settings.access_key",
			wantDetail: "access_key_secret is set, but access_key is missing or empty",
		},
		{
			name:       "ClearedSecret",
			key:        new("key"),
			secret:     new(""),
			wantField:  "settings.access_key_secret",
			wantDetail: "access_key is set, but access_key_secret is missing or empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			settings := codersdk.AIProviderSettings{Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				Model:           "model",
				SmallFastModel:  "small-model",
				AccessKey:       tc.key,
				AccessKeySecret: tc.secret,
			}}
			create := codersdk.CreateAIProviderRequest{
				Type: codersdk.AIProviderTypeBedrock, Name: "bedrock",
				BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com/", Settings: settings,
			}
			var want []codersdk.ValidationError
			if tc.wantField != "" {
				want = []codersdk.ValidationError{{
					Field: tc.wantField, Detail: tc.wantDetail,
				}}
			}
			require.Equal(t, want, create.Validate())
			require.Equal(t, want, settings.Bedrock.ValidateCredentials())

			update := codersdk.UpdateAIProviderRequest{Settings: &settings}
			if tc.key == nil || tc.secret == nil {
				// Omitted credentials are merged from storage by the API.
				require.Empty(t, update.Validate())
			} else {
				require.Equal(t, want, update.Validate())
			}
		})
	}
}

func TestAIProviderRequest_ValidateRoleARN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		roleARN string
		wantErr bool
	}{
		{name: "empty is allowed", roleARN: "", wantErr: false},
		{name: "standard role arn", roleARN: "arn:aws:iam::743809215448:role/bedrock-role", wantErr: false},
		{name: "govcloud partition", roleARN: "arn:aws-us-gov:iam::123456789012:role/bedrock-role", wantErr: false},
		{name: "china partition", roleARN: "arn:aws-cn:iam::123456789012:role/bedrock-role", wantErr: false},
		{name: "role path", roleARN: "arn:aws:iam::123456789012:role/team/bedrock-role", wantErr: false},
		{name: "not an arn", roleARN: "bedrock-role", wantErr: true},
		{name: "wrong resource type", roleARN: "arn:aws:iam::123456789012:user/dave", wantErr: true},
		{name: "wrong service", roleARN: "arn:aws:s3:::my-bucket", wantErr: true},
		{name: "truncated arn", roleARN: "arn:aws:iam::123456789012", wantErr: true},
	}

	hasRoleARNError := func(vs []codersdk.ValidationError) bool {
		for _, v := range vs {
			if v.Field == "settings.role_arn" {
				return true
			}
		}
		return false
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			settings := codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:  "us-east-1",
					RoleARN: tc.roleARN,
				},
			}

			create := codersdk.CreateAIProviderRequest{
				Type:     codersdk.AIProviderTypeBedrock,
				Name:     "bedrock",
				BaseURL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
				Settings: settings,
			}
			require.Equal(t, tc.wantErr, hasRoleARNError(create.Validate()))

			update := codersdk.UpdateAIProviderRequest{Settings: &settings}
			require.Equal(t, tc.wantErr, hasRoleARNError(update.Validate()))
		})
	}
}

func TestAIProviderRequest_ValidateBedrockProtocol(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		protocol codersdk.AIProviderBedrockProtocol
		wantErr  bool
	}{
		{name: "empty is allowed", protocol: "", wantErr: false},
		{name: "invoke-model", protocol: codersdk.AIProviderBedrockProtocolInvokeModel, wantErr: false},
		{name: "mantle", protocol: codersdk.AIProviderBedrockProtocolMantle, wantErr: false},
		{name: "typo", protocol: "mnatle", wantErr: true},
		{name: "unknown", protocol: "http", wantErr: true},
	}

	hasProtocolError := func(vs []codersdk.ValidationError) bool {
		for _, v := range vs {
			if v.Field == "settings.protocol" {
				return true
			}
		}
		return false
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			settings := codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Region:   "us-east-1",
					Protocol: tc.protocol,
				},
			}

			create := codersdk.CreateAIProviderRequest{
				Type:     codersdk.AIProviderTypeBedrock,
				Name:     "bedrock",
				BaseURL:  "https://bedrock-mantle.us-east-1.api.aws/anthropic",
				Settings: settings,
			}
			require.Equal(t, tc.wantErr, hasProtocolError(create.Validate()))

			update := codersdk.UpdateAIProviderRequest{Settings: &settings}
			require.Equal(t, tc.wantErr, hasProtocolError(update.Validate()))
		})
	}
}

func TestAIProviderRequest_ValidateBedrockMantle(t *testing.T) {
	t.Parallel()

	hasFieldError := func(vs []codersdk.ValidationError, field string) bool {
		for _, v := range vs {
			if v.Field == field {
				return true
			}
		}
		return false
	}

	t.Run("MantleRequiresRegion", func(t *testing.T) {
		t.Parallel()
		create := codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeBedrock,
			Name:    "bedrock",
			BaseURL: "https://bedrock-mantle.us-east-1.api.aws",
			Settings: codersdk.AIProviderSettings{
				Bedrock: &codersdk.AIProviderBedrockSettings{
					Protocol: codersdk.AIProviderBedrockProtocolMantle,
				},
			},
		}
		require.True(t, hasFieldError(create.Validate(), "settings.region"))

		create.Settings.Bedrock.Region = "us-east-1"
		require.False(t, hasFieldError(create.Validate(), "settings.region"))
	})

	t.Run("MantleRequiresRegionOnUpdate", func(t *testing.T) {
		t.Parallel()
		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Protocol: codersdk.AIProviderBedrockProtocolMantle,
			},
		}
		update := codersdk.UpdateAIProviderRequest{Settings: &settings}
		require.True(t, hasFieldError(update.Validate(), "settings.region"))

		settings.Bedrock.Region = "us-east-1"
		require.False(t, hasFieldError(update.Validate(), "settings.region"))
	})

	t.Run("InvokeModelDoesNotRequireRegionField", func(t *testing.T) {
		t.Parallel()
		// The mantle-specific region check must not fire for the invoke-model
		// protocol, whether it is set explicitly or left empty (existing rows).
		for _, protocol := range []codersdk.AIProviderBedrockProtocol{"", codersdk.AIProviderBedrockProtocolInvokeModel} {
			create := codersdk.CreateAIProviderRequest{
				Type:    codersdk.AIProviderTypeBedrock,
				Name:    "bedrock",
				BaseURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
				Settings: codersdk.AIProviderSettings{
					Bedrock: &codersdk.AIProviderBedrockSettings{Protocol: protocol},
				},
			}
			require.False(t, hasFieldError(create.Validate(), "settings.region"))
		}
	})
}

// TestAIProviderRequest_ValidationInSync keeps API-level validation
// (CreateAIProviderRequest.Validate) and runtime-level validation
// (config.AWSBedrock.Validate) in sync.
func TestAIProviderRequest_ValidationInSync(t *testing.T) {
	t.Parallel()

	const (
		model          = "anthropic.claude-sonnet-4-5"
		smallFastModel = "anthropic.claude-haiku-4-5"
	)

	cases := []struct {
		name    string
		baseURL string
		bedrock codersdk.AIProviderBedrockSettings
		isValid bool
	}{
		{
			name:    "InvokeModelRegionOnly",
			baseURL: "https://bedrock.us-east-2.amazonaws.com",
			bedrock: codersdk.AIProviderBedrockSettings{Region: "us-east-2"},
			isValid: false,
		},
		{
			name:    "InvokeModelMissingSmallFastModel",
			baseURL: "https://bedrock.us-east-2.amazonaws.com",
			bedrock: codersdk.AIProviderBedrockSettings{
				Region: "us-east-2",
				Model:  model,
			},
			isValid: false,
		},
		{
			name:    "InvokeModelComplete",
			baseURL: "https://bedrock.us-east-2.amazonaws.com",
			bedrock: codersdk.AIProviderBedrockSettings{
				Region:         "us-east-2",
				Model:          model,
				SmallFastModel: smallFastModel,
			},
			isValid: true,
		},
		{
			// Mantle forwards the client's model unchanged, so it needs neither.
			name:    "MantleWithoutModels",
			baseURL: "https://bedrock-mantle.us-east-2.api.aws/anthropic",
			bedrock: codersdk.AIProviderBedrockSettings{
				Region:   "us-east-2",
				Protocol: codersdk.AIProviderBedrockProtocolMantle,
			},
			isValid: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mirror the settings-to-runtime conversion that cli/aibridged.go
			// performs when it builds providers from the database.
			runtimeCfg := config.AWSBedrock{
				BaseURL:        tc.baseURL,
				Region:         tc.bedrock.Region,
				Model:          tc.bedrock.Model,
				SmallFastModel: tc.bedrock.SmallFastModel,
				Protocol:       config.BedrockProtocol(tc.bedrock.ResolvedProtocol()),
			}
			require.Equal(t, tc.isValid, runtimeCfg.Validate() == nil,
				"config.AWSBedrock.Validate disagrees with the expected verdict")

			create := codersdk.CreateAIProviderRequest{
				Type:     codersdk.AIProviderTypeBedrock,
				Name:     "bedrock",
				BaseURL:  tc.baseURL,
				Settings: codersdk.AIProviderSettings{Bedrock: &tc.bedrock},
			}
			require.Equal(t, tc.isValid, len(create.Validate()) == 0,
				"the API disagrees with the expected verdict")
		})
	}
}
