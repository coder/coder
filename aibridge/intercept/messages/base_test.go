package messages //nolint:testpackage // tests unexported internals

import (
	"context"
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/utils"
)

func TestScanForCorrelatingToolCallID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		requestBody string
		expected    *string
	}{
		{
			name:        "no messages field",
			requestBody: `{}`,
			expected:    nil,
		},
		{
			name:        "messages string",
			requestBody: `{"messages":"test"}`,
			expected:    nil,
		},
		{
			name:        "empty messages array",
			requestBody: `{"messages":[]}`,
			expected:    nil,
		},
		{
			name:        "last message has no tool result blocks",
			requestBody: `{"messages":[{"role":"user","content":"hello"}]}`,
			expected:    nil,
		},
		{
			name:        "single tool result block",
			requestBody: `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_abc","content":"result"}]}]}`,
			expected:    utils.PtrTo("toolu_abc"),
		},
		{
			name:        "multiple tool result blocks returns last",
			requestBody: `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_first","content":"first"},{"type":"text","text":"ignored"},{"type":"tool_result","tool_use_id":"toolu_second","content":"second"}]}]}`,
			expected:    utils.PtrTo("toolu_second"),
		},
		{
			name:        "last message is not a tool result",
			requestBody: `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_first","content":"first"}]},{"role":"user","content":"some text"}]}`,
			expected:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := &interceptionBase{
				reqPayload: mustMessagesPayload(t, tc.requestBody),
			}

			require.Equal(t, tc.expected, base.CorrelatingToolCallID())
		})
	}
}

func TestAWSBedrockValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         *config.AWSBedrock
		expectError bool
		errorMsg    string
	}{
		// Valid cases: static credentials.
		{
			name: "static credentials with region",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
		},
		{
			name: "static credentials with base url",
			cfg: &config.AWSBedrock{
				BaseURL:         "http://bedrock.internal",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
		},
		{
			// There unfortunately isn't a way for us to determine precedence in a unit test,
			// since the produced options take a `requestconfig.RequestConfig` input value
			// which is internal to the anthropic SDK.
			//
			// See TestAWSBedrockIntegration which validates this.
			name: "static credentials with base url & region",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
		},
		// Invalid cases.
		{
			name: "missing region & base url",
			cfg: &config.AWSBedrock{
				Region:          "",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
			expectError: true,
			errorMsg:    "region or base url required",
		},
		{
			name: "missing access key",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
			expectError: true,
			errorMsg:    "both access key and access key secret must be provided together",
		},
		{
			name: "missing access key secret",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKey:       "test-key",
				AccessKeySecret: "",
				Model:           "test-model",
				SmallFastModel:  "test-small-model",
			},
			expectError: true,
			errorMsg:    "both access key and access key secret must be provided together",
		},
		{
			name: "missing model",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "",
				SmallFastModel:  "test-small-model",
			},
			expectError: true,
			errorMsg:    "model required",
		},
		{
			name: "missing small fast model",
			cfg: &config.AWSBedrock{
				Region:          "us-east-1",
				AccessKey:       "test-key",
				AccessKeySecret: "test-secret",
				Model:           "test-model",
				SmallFastModel:  "",
			},
			expectError: true,
			errorMsg:    "small fast model required",
		},
		{
			name:        "all fields empty",
			cfg:         &config.AWSBedrock{},
			expectError: true,
			errorMsg:    "region or base url required",
		},
		{
			name:        "nil config",
			cfg:         nil,
			expectError: true,
			errorMsg:    "nil config given",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := &interceptionBase{}
			opts, err := base.withAWSBedrockOptions(context.Background(), tt.cfg)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NotEmpty(t, opts)
				require.NoError(t, err)
			}
		})
	}
}

// TestAWSBedrockCredentialChain tests credential resolution via the AWS SDK default credential chain.
// NOTE: Cannot use t.Parallel() here because subtests use t.Setenv which requires sequential execution.
func TestAWSBedrockCredentialChain(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.AWSBedrock
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "temporary credentials via env",
			cfg: &config.AWSBedrock{
				Region:         "us-east-1",
				Model:          "test-model",
				SmallFastModel: "test-small-model",
			},
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret",
			},
		},
		{
			name: "temporary credentials with session token via env",
			cfg: &config.AWSBedrock{
				Region:         "us-east-1",
				Model:          "test-model",
				SmallFastModel: "test-small-model",
			},
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret",
				"AWS_SESSION_TOKEN":     "test-session-token",
			},
		},
		{
			// When static credentials are not provided and no environment credentials are set,
			// the SDK default credential chain fails to resolve credentials.
			name: "error when no credential source is configured",
			cfg: &config.AWSBedrock{
				Region:         "us-east-1",
				Model:          "test-model",
				SmallFastModel: "test-small-model",
			},
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID":           "",
				"AWS_SECRET_ACCESS_KEY":       "",
				"AWS_SESSION_TOKEN":           "",
				"AWS_PROFILE":                 "",
				"AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
				"AWS_CONFIG_FILE":             "/dev/null",
			},
			expectError: true,
			errorMsg:    "no AWS credentials found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}
			base := &interceptionBase{}
			opts, err := base.withAWSBedrockOptions(context.Background(), tt.cfg)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NotEmpty(t, opts)
				require.NoError(t, err)
			}
		})
	}
}

func TestAccumulateUsage(t *testing.T) {
	t.Parallel()

	t.Run("Usage to Usage", func(t *testing.T) {
		t.Parallel()
		dest := &anthropic.Usage{
			InputTokens:              10,
			OutputTokens:             20,
			CacheCreationInputTokens: 5,
			CacheReadInputTokens:     3,
			CacheCreation: anthropic.CacheCreation{
				Ephemeral1hInputTokens: 2,
				Ephemeral5mInputTokens: 1,
			},
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 1,
			},
		}

		source := anthropic.Usage{
			InputTokens:              15,
			OutputTokens:             25,
			CacheCreationInputTokens: 8,
			CacheReadInputTokens:     4,
			CacheCreation: anthropic.CacheCreation{
				Ephemeral1hInputTokens: 3,
				Ephemeral5mInputTokens: 2,
			},
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 2,
			},
		}

		accumulateUsage(dest, source)

		require.EqualValues(t, 25, dest.InputTokens)
		require.EqualValues(t, 45, dest.OutputTokens)
		require.EqualValues(t, 13, dest.CacheCreationInputTokens)
		require.EqualValues(t, 7, dest.CacheReadInputTokens)
		require.EqualValues(t, 5, dest.CacheCreation.Ephemeral1hInputTokens)
		require.EqualValues(t, 3, dest.CacheCreation.Ephemeral5mInputTokens)
		require.EqualValues(t, 3, dest.ServerToolUse.WebSearchRequests)
	})

	t.Run("MessageDeltaUsage to MessageDeltaUsage", func(t *testing.T) {
		t.Parallel()

		dest := &anthropic.MessageDeltaUsage{
			InputTokens:              10,
			OutputTokens:             20,
			CacheCreationInputTokens: 5,
			CacheReadInputTokens:     3,
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 1,
			},
		}

		source := anthropic.MessageDeltaUsage{
			InputTokens:              15,
			OutputTokens:             25,
			CacheCreationInputTokens: 8,
			CacheReadInputTokens:     4,
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 2,
			},
		}

		accumulateUsage(dest, source)

		require.EqualValues(t, 25, dest.InputTokens)
		require.EqualValues(t, 45, dest.OutputTokens)
		require.EqualValues(t, 13, dest.CacheCreationInputTokens)
		require.EqualValues(t, 7, dest.CacheReadInputTokens)
		require.EqualValues(t, 3, dest.ServerToolUse.WebSearchRequests)
	})

	t.Run("Usage to MessageDeltaUsage", func(t *testing.T) {
		t.Parallel()

		dest := &anthropic.MessageDeltaUsage{
			InputTokens:              10,
			OutputTokens:             20,
			CacheCreationInputTokens: 5,
			CacheReadInputTokens:     3,
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 1,
			},
		}

		source := anthropic.Usage{
			InputTokens:              15,
			OutputTokens:             25,
			CacheCreationInputTokens: 8,
			CacheReadInputTokens:     4,
			CacheCreation: anthropic.CacheCreation{
				Ephemeral1hInputTokens: 3, // These won't be accumulated to MessageDeltaUsage
				Ephemeral5mInputTokens: 2,
			},
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 2,
			},
		}

		accumulateUsage(dest, source)

		require.EqualValues(t, 25, dest.InputTokens)
		require.EqualValues(t, 45, dest.OutputTokens)
		require.EqualValues(t, 13, dest.CacheCreationInputTokens)
		require.EqualValues(t, 7, dest.CacheReadInputTokens)
		require.EqualValues(t, 3, dest.ServerToolUse.WebSearchRequests)
	})

	t.Run("MessageDeltaUsage to Usage", func(t *testing.T) {
		t.Parallel()

		dest := &anthropic.Usage{
			InputTokens:              10,
			OutputTokens:             20,
			CacheCreationInputTokens: 5,
			CacheReadInputTokens:     3,
			CacheCreation: anthropic.CacheCreation{
				Ephemeral1hInputTokens: 2,
				Ephemeral5mInputTokens: 1,
			},
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 1,
			},
		}

		source := anthropic.MessageDeltaUsage{
			InputTokens:              15,
			OutputTokens:             25,
			CacheCreationInputTokens: 8,
			CacheReadInputTokens:     4,
			ServerToolUse: anthropic.ServerToolUsage{
				WebSearchRequests: 2,
			},
		}

		accumulateUsage(dest, source)

		require.EqualValues(t, 25, dest.InputTokens)
		require.EqualValues(t, 45, dest.OutputTokens)
		require.EqualValues(t, 13, dest.CacheCreationInputTokens)
		require.EqualValues(t, 7, dest.CacheReadInputTokens)
		// Ephemeral tokens remain unchanged since MessageDeltaUsage doesn't have them
		require.EqualValues(t, 2, dest.CacheCreation.Ephemeral1hInputTokens)
		require.EqualValues(t, 1, dest.CacheCreation.Ephemeral5mInputTokens)
		require.EqualValues(t, 3, dest.ServerToolUse.WebSearchRequests)
	})

	t.Run("Nil or unsupported types", func(t *testing.T) {
		t.Parallel()

		// Test with nil dest
		var nilUsage *anthropic.Usage
		source := anthropic.Usage{InputTokens: 10}
		accumulateUsage(nilUsage, source) // Should not panic

		// Test with unsupported types
		var unsupported string
		accumulateUsage(&unsupported, source) // Should not panic, just do nothing
	})
}

func TestInjectTools_CacheBreakpoints(t *testing.T) {
	t.Parallel()

	t.Run("cache control preserved when no tools to inject", func(t *testing.T) {
		t.Parallel()

		// Request has existing tool with cache control, but no tools to inject.
		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tools":[`+
				`{"name":"existing_tool","type":"custom","input_schema":{"type":"object","properties":{}},"cache_control":{"type":"ephemeral"}}]}`),
			mcpProxy: &mockServerProxier{tools: nil},
			logger:   slog.Make(),
		}

		i.injectTools()

		// Cache control should remain untouched since no tools were injected.
		toolItems := gjson.GetBytes(i.reqPayload, "tools").Array()
		require.Len(t, toolItems, 1)
		require.Equal(t, "existing_tool", toolItems[0].Get("name").String())
		require.Equal(t, string(constant.ValueOf[constant.Ephemeral]()), toolItems[0].Get("cache_control.type").String())
	})

	t.Run("cache control breakpoint is preserved by prepending injected tools", func(t *testing.T) {
		t.Parallel()

		// Request has existing tool with cache control.
		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tools":[`+
				`{"name":"existing_tool","type":"custom","input_schema":{"type":"object","properties":{}},"cache_control":{"type":"ephemeral"}}]}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{
					{ID: "injected_tool", Name: "injected", Description: "Injected tool"},
				},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolItems := gjson.GetBytes(i.reqPayload, "tools").Array()
		require.Len(t, toolItems, 2)
		// Injected tools are prepended.
		require.Equal(t, "injected_tool", toolItems[0].Get("name").String())
		require.Empty(t, toolItems[0].Get("cache_control.type").String())
		// Original tool's cache control should be preserved at the end.
		require.Equal(t, "existing_tool", toolItems[1].Get("name").String())
		require.Equal(t, string(constant.ValueOf[constant.Ephemeral]()), toolItems[1].Get("cache_control.type").String())
	})

	// The cache breakpoint SHOULD be on the final tool, but may not be; we must preserve that intention.
	t.Run("cache control breakpoint in non-standard location is preserved", func(t *testing.T) {
		t.Parallel()

		// Request has multiple tools with cache control breakpoints.
		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tools":[`+
				`{"name":"tool_with_cache_1","type":"custom","input_schema":{"type":"object","properties":{}},"cache_control":{"type":"ephemeral"}},`+
				`{"name":"tool_with_cache_2","type":"custom","input_schema":{"type":"object","properties":{}}}]}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{
					{ID: "injected_tool", Name: "injected", Description: "Injected tool"},
				},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolItems := gjson.GetBytes(i.reqPayload, "tools").Array()
		require.Len(t, toolItems, 3)
		// Injected tool is prepended without cache control.
		require.Equal(t, "injected_tool", toolItems[0].Get("name").String())
		require.Empty(t, toolItems[0].Get("cache_control.type").String())
		// Both original tools' cache controls should remain.
		require.Equal(t, "tool_with_cache_1", toolItems[1].Get("name").String())
		require.Equal(t, string(constant.ValueOf[constant.Ephemeral]()), toolItems[1].Get("cache_control.type").String())
		require.Equal(t, "tool_with_cache_2", toolItems[2].Get("name").String())
		require.Empty(t, toolItems[2].Get("cache_control.type").String())
	})

	t.Run("no cache control added when none originally set", func(t *testing.T) {
		t.Parallel()

		// Request has tools but none with cache control.
		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tools":[`+
				`{"name":"existing_tool_no_cache","type":"custom","input_schema":{"type":"object","properties":{}}}]}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{
					{ID: "injected_tool", Name: "injected", Description: "Injected tool"},
				},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolItems := gjson.GetBytes(i.reqPayload, "tools").Array()
		require.Len(t, toolItems, 2)
		// Injected tool is prepended without cache control.
		require.Equal(t, "injected_tool", toolItems[0].Get("name").String())
		require.Empty(t, toolItems[0].Get("cache_control.type").String())
		// Original tool remains at the end without cache control.
		require.Equal(t, "existing_tool_no_cache", toolItems[1].Get("name").String())
		require.Empty(t, toolItems[1].Get("cache_control.type").String())
	})
}

func TestInjectTools_ParallelToolCalls(t *testing.T) {
	t.Parallel()

	t.Run("does not modify tool choice when no tools to inject", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tool_choice":{"type":"auto"}}`),
			mcpProxy:   &mockServerProxier{tools: nil}, // No tools to inject.
			logger:     slog.Make(),
		}

		i.injectTools()

		// Tool choice should remain unchanged - DisableParallelToolUse should not be set.
		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.Auto]()), toolChoice.Get("type").String())
		require.False(t, toolChoice.Get("disable_parallel_tool_use").Exists())
	})

	t.Run("disables parallel tool use for empty tool choice (default)", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{{ID: "test_tool", Name: "test", Description: "Test"}},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.Auto]()), toolChoice.Get("type").String())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Exists())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Bool())
	})

	t.Run("disables parallel tool use for explicit auto tool choice", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tool_choice":{"type":"auto"}}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{{ID: "test_tool", Name: "test", Description: "Test"}},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.Auto]()), toolChoice.Get("type").String())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Exists())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Bool())
	})

	t.Run("disables parallel tool use for any tool choice", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tool_choice":{"type":"any"}}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{{ID: "test_tool", Name: "test", Description: "Test"}},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.Any]()), toolChoice.Get("type").String())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Exists())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Bool())
	})

	t.Run("disables parallel tool use for tool choice type", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tool_choice":{"type":"tool","name":"specific_tool"}}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{{ID: "test_tool", Name: "test", Description: "Test"}},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.Tool]()), toolChoice.Get("type").String())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Exists())
		require.True(t, toolChoice.Get("disable_parallel_tool_use").Bool())
	})

	t.Run("no-op for none tool choice type", func(t *testing.T) {
		t.Parallel()

		i := &interceptionBase{
			reqPayload: mustMessagesPayload(t, `{"tool_choice":{"type":"none"}}`),
			mcpProxy: &mockServerProxier{
				tools: []*mcp.Tool{{ID: "test_tool", Name: "test", Description: "Test"}},
			},
			logger: slog.Make(),
		}

		i.injectTools()

		// Tools are still injected.
		require.Len(t, gjson.GetBytes(i.reqPayload, "tools").Array(), 1)
		// But no parallel tool use modification for "none" type.
		toolChoice := gjson.GetBytes(i.reqPayload, "tool_choice")
		require.Equal(t, string(constant.ValueOf[constant.None]()), toolChoice.Get("type").String())
		require.False(t, toolChoice.Get("disable_parallel_tool_use").Exists())
	})
}

func TestAugmentRequestForBedrock_AdaptiveThinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		bedrockModel    string
		requestBody     string
		clientBetaFlags string

		expectThinkingType  string
		expectBudgetTokens  int64 // 0 means budget_tokens should not be present
		expectRemovedFields []string
		expectKeptFields    []string
		expectBetaValues    []string // expected separate Anthropic-Beta header values
	}{
		{
			name:               "non_4_6_model_with_adaptive_thinking_gets_converted",
			bedrockModel:       "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:        `{"max_tokens":10000,"thinking":{"type":"adaptive"}}`,
			expectThinkingType: "enabled",
			expectBudgetTokens: 8000, // 10000 * 0.8 (default/high effort)
		},
		{
			name:               "non_4_6_model_with_adaptive_thinking_and_small_max_tokens_disables_thinking",
			bedrockModel:       "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:        `{"max_tokens":1000,"thinking":{"type":"adaptive"}}`,
			expectThinkingType: "disabled",
		},
		{
			name:               "opus_4_6_model_with_adaptive_thinking_is_not_converted",
			bedrockModel:       "anthropic.claude-opus-4-6-v1",
			requestBody:        `{"max_tokens":10000,"thinking":{"type":"adaptive"}}`,
			expectThinkingType: "adaptive",
		},
		{
			name:               "sonnet_4_6_model_with_adaptive_thinking_is_not_converted",
			bedrockModel:       "anthropic.claude-sonnet-4-6",
			requestBody:        `{"max_tokens":10000,"thinking":{"type":"adaptive"}}`,
			expectThinkingType: "adaptive",
		},
		{
			name:         "non_4_6_model_with_no_thinking_field_is_unchanged",
			bedrockModel: "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:  `{"max_tokens":10000}`,
		},
		{
			name:               "non_4_6_model_with_enabled_thinking_is_unchanged",
			bedrockModel:       "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:        `{"max_tokens":10000,"thinking":{"type":"enabled","budget_tokens":5000}}`,
			expectThinkingType: "enabled",
			expectBudgetTokens: 5000,
		},
		{
			name:                "output_config_stripped_without_beta_flag_and_effort_used_for_budget",
			bedrockModel:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:         `{"max_tokens":10000,"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`,
			expectThinkingType:  "enabled",
			expectBudgetTokens:  2000, // 10000 * 0.2 (low effort)
			expectRemovedFields: []string{"output_config"},
		},
		{
			name:             "output_config_kept_when_effort_beta_flag_present_on_opus_4_5",
			bedrockModel:     "anthropic.claude-opus-4-5-20250929-v1:0",
			clientBetaFlags:  "effort-2025-11-24,interleaved-thinking-2025-05-14",
			requestBody:      `{"max_tokens":10000,"output_config":{"effort":"high"}}`,
			expectKeptFields: []string{"output_config"},
			expectBetaValues: []string{"effort-2025-11-24", "interleaved-thinking-2025-05-14"},
		},
		{
			name:                "output_config_stripped_for_non_opus_4_5_even_with_effort_beta_flag",
			bedrockModel:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			clientBetaFlags:     "effort-2025-11-24,interleaved-thinking-2025-05-14",
			requestBody:         `{"max_tokens":10000,"output_config":{"effort":"high"}}`,
			expectRemovedFields: []string{"output_config"},
			expectBetaValues:    []string{"interleaved-thinking-2025-05-14"},
		},
		{
			name:             "context_management_kept_when_beta_flag_present",
			bedrockModel:     "anthropic.claude-sonnet-4-5-20250929-v1:0",
			clientBetaFlags:  "context-management-2025-06-27",
			requestBody:      `{"max_tokens":10000,"context_management":{"type":"auto"}}`,
			expectKeptFields: []string{"context_management"},
			expectBetaValues: []string{"context-management-2025-06-27"},
		},
		{
			name:                "context_management_stripped_without_beta_flag",
			bedrockModel:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			requestBody:         `{"max_tokens":10000,"context_management":{"type":"auto"}}`,
			expectRemovedFields: []string{"context_management"},
		},
		{
			name:                "context_management_stripped_for_unsupported_model_even_with_beta_flag",
			bedrockModel:        "anthropic.claude-opus-4-6-v1",
			clientBetaFlags:     "context-management-2025-06-27",
			requestBody:         `{"max_tokens":10000,"thinking":{"type":"adaptive"},"context_management":{"type":"auto"}}`,
			expectThinkingType:  "adaptive",
			expectRemovedFields: []string{"context_management"},
		},
		{
			name:             "unsupported_beta_flags_are_filtered_out",
			bedrockModel:     "anthropic.claude-sonnet-4-5-20250929-v1:0",
			clientBetaFlags:  "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05",
			requestBody:      `{"max_tokens":10000}`,
			expectBetaValues: []string{"interleaved-thinking-2025-05-14"},
		},
		{
			name:                "all_unsupported_fields_stripped_and_beta_flags_filtered",
			bedrockModel:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			clientBetaFlags:     "claude-code-20250219,prompt-caching-scope-2026-01-05",
			requestBody:         `{"max_tokens":10000,"output_config":{"effort":"high"},"metadata":{"user_id":"u123"},"service_tier":"auto","container":"ctr_abc","inference_geo":"us","context_management":{"type":"auto"}}`,
			expectRemovedFields: []string{"output_config", "metadata", "service_tier", "container", "inference_geo", "context_management"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var clientHeaders http.Header
			if tc.clientBetaFlags != "" {
				clientHeaders = http.Header{
					"Anthropic-Beta": {tc.clientBetaFlags},
				}
			}

			i := &interceptionBase{
				reqPayload: mustMessagesPayload(t, tc.requestBody),
				bedrockCfg: &config.AWSBedrock{
					Model:          tc.bedrockModel,
					SmallFastModel: "anthropic.claude-haiku-3-5",
				},
				clientHeaders: clientHeaders,
				logger:        slog.Make(),
			}

			i.augmentRequestForBedrock()

			thinkingType := gjson.GetBytes(i.reqPayload, "thinking.type")
			if tc.expectThinkingType == "" {
				require.False(t, thinkingType.Exists())
			} else {
				require.Equal(t, tc.expectThinkingType, thinkingType.String())
			}

			budgetTokens := gjson.GetBytes(i.reqPayload, "thinking.budget_tokens")
			if tc.expectBudgetTokens == 0 {
				require.False(t, budgetTokens.Exists(), "budget_tokens should not be set")
			} else {
				require.Equal(t, tc.expectBudgetTokens, budgetTokens.Int())
			}

			// Model should always be set to the bedrock model.
			require.Equal(t, tc.bedrockModel, gjson.GetBytes(i.reqPayload, "model").String())

			// Verify expected fields are removed.
			for _, field := range tc.expectRemovedFields {
				require.False(t, gjson.GetBytes(i.reqPayload, field).Exists(), "%s should be removed", field)
			}

			// Verify expected fields are kept.
			for _, field := range tc.expectKeptFields {
				require.True(t, gjson.GetBytes(i.reqPayload, field).Exists(), "%s should be kept", field)
			}

			got := clientHeaders.Values("Anthropic-Beta")
			require.Equal(t, tc.expectBetaValues, got)
		})
	}
}

func mustMessagesPayload(t *testing.T, requestBody string) RequestPayload {
	t.Helper()

	payload, err := NewRequestPayload([]byte(requestBody))
	require.NoError(t, err)

	return payload
}

// mockServerProxier is a test implementation of mcp.ServerProxier.
type mockServerProxier struct {
	tools []*mcp.Tool
}

func (*mockServerProxier) Init(context.Context) error {
	return nil
}

func (*mockServerProxier) Shutdown(context.Context) error {
	return nil
}

func (m *mockServerProxier) ListTools() []*mcp.Tool {
	return m.tools
}

func (m *mockServerProxier) GetTool(id string) *mcp.Tool {
	for _, t := range m.tools {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (*mockServerProxier) CallTool(context.Context, string, any) (*mcpgo.CallToolResult, error) {
	return nil, nil //nolint:nilnil // mock: no-op implementation
}

func TestFilterBedrockBetaFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		model        string
		inputValues  []string // header values to set (each element is a separate header value)
		expectValues []string // expected separate header values after filtering
	}{
		{
			name:         "empty header",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  nil,
			expectValues: nil,
		},
		{
			name:         "all supported flags kept",
			model:        "anthropic.claude-opus-4-5-20250929-v1:0",
			inputValues:  []string{"interleaved-thinking-2025-05-14,effort-2025-11-24"},
			expectValues: []string{"interleaved-thinking-2025-05-14", "effort-2025-11-24"},
		},
		{
			name:         "unsupported flags removed",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  []string{"claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05"},
			expectValues: []string{"interleaved-thinking-2025-05-14"},
		},
		{
			name:         "header removed when all flags unsupported",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  []string{"claude-code-20250219,prompt-caching-scope-2026-01-05"},
			expectValues: nil,
		},
		{
			name:         "effort flag removed for non opus 4.5 model",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  []string{"effort-2025-11-24,interleaved-thinking-2025-05-14"},
			expectValues: []string{"interleaved-thinking-2025-05-14"},
		},
		{
			name:         "effort flag kept for opus 4.5 model",
			model:        "anthropic.claude-opus-4-5-20250929-v1:0",
			inputValues:  []string{"effort-2025-11-24,interleaved-thinking-2025-05-14"},
			expectValues: []string{"effort-2025-11-24", "interleaved-thinking-2025-05-14"},
		},
		{
			name:         "context management kept for sonnet 4.5",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  []string{"context-management-2025-06-27"},
			expectValues: []string{"context-management-2025-06-27"},
		},
		{
			name:         "context management kept for haiku 4.5",
			model:        "anthropic.claude-haiku-4-5-20250929-v1:0",
			inputValues:  []string{"context-management-2025-06-27"},
			expectValues: []string{"context-management-2025-06-27"},
		},
		{
			name:         "context management removed for unsupported model",
			model:        "anthropic.claude-opus-4-6-v1",
			inputValues:  []string{"context-management-2025-06-27,interleaved-thinking-2025-05-14"},
			expectValues: []string{"interleaved-thinking-2025-05-14"},
		},
		{
			name:         "separate header values are handled correctly",
			model:        "anthropic.claude-sonnet-4-5-20250929-v1:0",
			inputValues:  []string{"interleaved-thinking-2025-05-14", "context-management-2025-06-27"},
			expectValues: []string{"interleaved-thinking-2025-05-14", "context-management-2025-06-27"},
		},
		{
			name:         "mixed comma-joined and separate header values",
			model:        "anthropic.claude-opus-4-5-20250929-v1:0",
			inputValues:  []string{"interleaved-thinking-2025-05-14,effort-2025-11-24", "token-efficient-tools-2025-02-19"},
			expectValues: []string{"interleaved-thinking-2025-05-14", "effort-2025-11-24", "token-efficient-tools-2025-02-19"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			for _, v := range tc.inputValues {
				headers.Add("Anthropic-Beta", v)
			}

			filterBedrockBetaFlags(headers, tc.model)

			// Each kept flag should be a separate header value.
			got := headers.Values("Anthropic-Beta")
			require.Equal(t, tc.expectValues, got)
		})
	}
}
