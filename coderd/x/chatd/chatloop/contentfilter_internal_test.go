package chatloop

import (
	"testing"

	fantasy "charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

func TestContentFilterError(t *testing.T) {
	t.Parallel()

	t.Run("WithRefusalMetadata", func(t *testing.T) {
		t.Parallel()
		meta := fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.RefusalMetadata{
				Category:    "cyber",
				Explanation: "blocked under policy",
			},
		}
		err := contentFilterError("anthropic", meta)
		classified := chaterror.Classify(err)
		if classified.Kind != codersdk.ChatErrorKindContentFilter {
			t.Errorf("kind = %q, want content_filter", classified.Kind)
		}
		if classified.Message != "blocked under policy" {
			t.Errorf("message = %q, want explanation", classified.Message)
		}
	})

	t.Run("WithoutMetadataUsesDefault", func(t *testing.T) {
		t.Parallel()
		err := contentFilterError("anthropic", nil)
		classified := chaterror.Classify(err)
		if classified.Kind != codersdk.ChatErrorKindContentFilter {
			t.Errorf("kind = %q, want content_filter", classified.Kind)
		}
		if classified.Message == "" {
			t.Error("expected a default message, got empty")
		}
	})
}
