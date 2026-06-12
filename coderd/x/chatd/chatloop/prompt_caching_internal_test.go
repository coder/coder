package chatloop

import (
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

// Regression test: Bedrock-served Claude models must get Anthropic prompt
// caching. fantasy's bedrock provider reports Provider() == "bedrock", which a
// strict == fantasyanthropic.Name gate wrongly excluded, so cache_control was
// never emitted and every Bedrock chat re-billed its prefix at full price.
func TestShouldApplyAnthropicPromptCaching(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider string
		want     bool
	}{
		{"anthropic", fantasyanthropic.Name, true},
		{"bedrock", fantasybedrock.Name, true},
		{"openai", "openai", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			model := &chattest.FakeModel{ProviderName: tc.provider}
			if got := shouldApplyAnthropicPromptCaching(model); got != tc.want {
				t.Errorf("shouldApplyAnthropicPromptCaching(provider=%q) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}

	if shouldApplyAnthropicPromptCaching(nil) {
		t.Error("shouldApplyAnthropicPromptCaching(nil) = true, want false")
	}
}
