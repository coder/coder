package codersdk_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// wifTestPath builds a platform-absolute path for identity token file
// fixtures: the WIF trust check requires absolute paths, and
// filepath.IsAbs rejects Unix-style paths on Windows.
func wifTestPath(parts ...string) string {
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestAIProviderSettings_Marshal(t *testing.T) {
	t.Parallel()

	t.Run("EmptyEmitsNull", func(t *testing.T) {
		t.Parallel()
		// The zero value marshals to JSON null so provider responses
		// keep the shape earlier clients decode; the {} clear form is
		// request-only and must be sent as raw JSON.
		got, err := json.Marshal(codersdk.AIProviderSettings{})
		require.NoError(t, err)
		require.Equal(t, `null`, string(got))
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

	t.Run("EmptyObjectClears", func(t *testing.T) {
		t.Parallel()
		// A literal {} is the explicit clear form used by PATCH callers,
		// e.g. migrating a WIF provider back to bearer keys. Only the
		// field-free object qualifies; see MissingTypeDiscriminator for
		// the typo'd-payload case that must stay an error.
		s := codersdk.AIProviderSettings{
			WIF: &codersdk.AIProviderWIFSettings{FederationRuleID: "fdrl_test"},
		}
		require.NoError(t, json.Unmarshal([]byte(`{}`), &s))
		require.True(t, s.IsZero())
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

func TestValidateAIProviderWIFBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{name: "empty uses default https endpoint", baseURL: "", wantErr: false},
		{name: "https allowed", baseURL: "https://proxy.example/api", wantErr: false},
		{name: "loopback http allowed", baseURL: "http://localhost:8080", wantErr: false},
		{name: "loopback ipv4 allowed", baseURL: "http://127.0.0.1:8080", wantErr: false},
		{name: "cleartext http rejected", baseURL: "http://proxy.example/api", wantErr: true},
		// Malformed URLs are left to the general base_url validation.
		{name: "malformed deferred", baseURL: "://", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verrs := codersdk.ValidateAIProviderWIFBaseURL(tc.baseURL)
			if !tc.wantErr {
				require.Empty(t, verrs)
				return
			}
			require.NotEmpty(t, verrs)
			require.Equal(t, "base_url", verrs[0].Field)
			require.Contains(t, verrs[0].Detail, "https base_url")
		})
	}
}

func TestAIBridgeConfigWIFIdentityTokenFileAllowed(t *testing.T) {
	t.Parallel()

	allowedToken := wifTestPath("var", "run", "secrets", "allowed", "token")
	envToken := wifTestPath("var", "run", "secrets", "env", "token")
	// A dot-dot spelling of allowedToken that filepath.Clean collapses
	// back to it.
	allowedTokenDotDot := wifTestPath("var", "run", "secrets", "allowed") +
		string(filepath.Separator) + filepath.Join("..", "allowed", "token")

	cfg := codersdk.AIBridgeConfig{
		// The empty entry must be ignored: filepath.Clean("") is ".",
		// which must never match anything.
		WIFAllowedIdentityTokenFiles: serpent.StringArray{allowedToken, "", "relative/entry"},
		Providers: []codersdk.AIProviderConfig{
			{
				Type:                 "anthropic",
				Name:                 "env-wif",
				BaseURL:              "https://gateway.internal/anthropic",
				WIFIdentityTokenFile: envToken,
			},
			{
				Type: "anthropic",
				Name: "env-keyed",
			},
		},
	}

	tests := []struct {
		name    string
		file    string
		baseURL string
		want    bool
	}{
		{name: "allowlisted any base URL", file: allowedToken, baseURL: "https://attacker.example", want: true},
		{name: "allowlisted dot-dot normalized", file: allowedTokenDotDot, baseURL: "https://api.anthropic.com", want: true},
		{name: "relative candidate rejected", file: "var/run/secrets/allowed/token", baseURL: "https://api.anthropic.com", want: false},
		{name: "relative allowlist entry ignored", file: "relative/entry", baseURL: "https://api.anthropic.com", want: false},
		{name: "empty candidate rejected", file: "", baseURL: "https://api.anthropic.com", want: false},
		{name: "dot candidate rejected", file: ".", baseURL: "https://api.anthropic.com", want: false},
		{name: "unlisted file rejected", file: wifTestPath("etc", "coder", "secret.pem"), baseURL: "https://api.anthropic.com", want: false},
		{name: "env pair matches", file: envToken, baseURL: "https://gateway.internal/anthropic", want: true},
		{name: "env file with different base URL rejected", file: envToken, baseURL: "https://attacker.example", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, cfg.WIFIdentityTokenFileAllowed(tc.file, tc.baseURL))
		})
	}

	t.Run("zero config rejects everything", func(t *testing.T) {
		t.Parallel()
		require.False(t, codersdk.AIBridgeConfig{}.WIFIdentityTokenFileAllowed(allowedToken, "https://api.anthropic.com"))
	})
}
