package bedrocksig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept/bedrocksig"
)

func TestBaseURLForModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		base    string
		model   string
		want    string
		wantErr bool
	}{
		// anthropic.* models route to the /anthropic prefix; the
		// anthropic-go SDK appends /v1/messages itself.
		{name: "anthropic from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "anthropic.claude-sonnet-5", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},
		{name: "anthropic keeps stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "anthropic.claude-opus-4-8", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},

		// openai.* models route to /openai/v1; the openai-go SDK appends
		// /responses or /chat/completions.
		{name: "openai from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "openai.gpt-5.6-luna", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "openai trims stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "openai.gpt-5.5", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "openai keeps stored /openai/v1", base: "https://bedrock-mantle.us-east-1.api.aws/openai/v1", model: "openai.gpt-5.4", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},

		// Third-party models use the root /v1 prefix; Mantle serves them on
		// /v1/chat/completions only.
		{name: "third-party from bare host", base: "https://bedrock-mantle.us-east-1.api.aws", model: "mistral.ministral-3-3b-instruct", want: "https://bedrock-mantle.us-east-1.api.aws/v1"},
		{name: "third-party trims stored /anthropic", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic", model: "moonshotai.kimi-k2.5", want: "https://bedrock-mantle.us-east-1.api.aws/v1"},

		// Trailing slashes and deeper stored prefixes are trimmed first.
		{name: "trailing slash", base: "https://bedrock-mantle.us-east-1.api.aws/", model: "openai.gpt-5.5", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},
		{name: "stored /anthropic/v1", base: "https://bedrock-mantle.us-east-1.api.aws/anthropic/v1", model: "anthropic.claude-sonnet-5", want: "https://bedrock-mantle.us-east-1.api.aws/anthropic"},
		{name: "stored /openai", base: "https://bedrock-mantle.us-east-1.api.aws/openai", model: "openai.gpt-5.4", want: "https://bedrock-mantle.us-east-1.api.aws/openai/v1"},

		// Non-vendor path segments survive trimming.
		{name: "proxy prefix preserved", base: "https://proxy.example.com/mantle", model: "openai.gpt-5.4", want: "https://proxy.example.com/mantle/openai/v1"},

		{name: "invalid base URL", base: "://not-a-url", model: "openai.gpt-5.4", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := bedrocksig.BaseURLForModel(tc.base, tc.model)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
