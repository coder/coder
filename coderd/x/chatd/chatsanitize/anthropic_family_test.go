package chatsanitize_test

import (
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"

	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
)

func TestIsAnthropicFamily(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider string
		want     bool
	}{
		// fantasy's bedrock provider wraps Anthropic but reports "bedrock";
		// both speak the Anthropic Messages wire format.
		{fantasyanthropic.Name, true},
		{fantasybedrock.Name, true},
		{"anthropic", true},
		{"bedrock", true},
		{"openai", false},
		{"google", false},
		{"azure", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := chatsanitize.IsAnthropicFamily(tc.provider); got != tc.want {
			t.Errorf("IsAnthropicFamily(%q) = %v, want %v", tc.provider, got, tc.want)
		}
	}
}
