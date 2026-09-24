package chatopenai_test

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

func TestWebSearchToolDisabled(t *testing.T) {
	t.Parallel()

	disabled := false

	tests := []struct {
		name    string
		options *codersdk.ChatModelOpenAIProviderOptions
	}{
		{
			name: "NilOptions",
		},
		{
			name:    "NilWebSearchEnabled",
			options: &codersdk.ChatModelOpenAIProviderOptions{},
		},
		{
			name: "WebSearchDisabled",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				WebSearchEnabled: &disabled,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool, ok := chatopenai.WebSearchTool(tt.options)
			require.False(t, ok)
			require.Nil(t, tool)
		})
	}
}

func TestWebSearchTool(t *testing.T) {
	t.Parallel()

	enabled := true
	searchContextSize := "high"
	allowedDomains := []string{"example.com", "coder.com"}

	tests := []struct {
		name    string
		options *codersdk.ChatModelOpenAIProviderOptions
		want    map[string]any
	}{
		{
			name: "NoExtraFields",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				WebSearchEnabled: &enabled,
			},
			want: map[string]any{},
		},
		{
			name: "SearchContextSize",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				WebSearchEnabled:  &enabled,
				SearchContextSize: &searchContextSize,
			},
			want: map[string]any{
				"search_context_size": searchContextSize,
			},
		},
		{
			name: "AllowedDomains",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				WebSearchEnabled: &enabled,
				AllowedDomains:   allowedDomains,
			},
			want: map[string]any{
				"allowed_domains": allowedDomains,
			},
		},
		{
			name: "BothFields",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				WebSearchEnabled:  &enabled,
				SearchContextSize: &searchContextSize,
				AllowedDomains:    allowedDomains,
			},
			want: map[string]any{
				"search_context_size": searchContextSize,
				"allowed_domains":     allowedDomains,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool, ok := chatopenai.WebSearchTool(tt.options)
			require.True(t, ok)

			providerTool, ok := tool.(fantasy.ProviderDefinedTool)
			require.True(t, ok)
			require.Equal(t, "web_search", providerTool.ID)
			require.Equal(t, "web_search", providerTool.Name)
			require.NotNil(t, providerTool.Args)
			require.Equal(t, tt.want, providerTool.Args)
		})
	}
}

func TestWebSearchResultJSON(t *testing.T) {
	t.Parallel()

	openAIMetadata := func(action *fantasyopenai.WebSearchAction) fantasy.ProviderMetadata {
		return fantasy.ProviderMetadata{
			fantasyopenai.Name: &fantasyopenai.WebSearchCallMetadata{ItemID: "ws_1", Action: action},
		}
	}
	sources := []fantasyopenai.WebSearchSource{
		{Type: "url", URL: "https://coder.com/docs"},
		{Type: "url", URL: ""},
		{Type: "url", URL: "https://example.com/"},
	}

	tests := []struct {
		name     string
		metadata fantasy.ProviderMetadata
		want     string
	}{
		{
			name: "Sources",
			metadata: openAIMetadata(&fantasyopenai.WebSearchAction{
				Type:    "search",
				Queries: []string{"coder agents"},
				Query:   "coder agents",
				Sources: sources,
			}),
			want: `{"sources":[{"url":"https://coder.com/docs"},{"url":"https://example.com/"}]}`,
		},
		{
			name: "NoSources",
			metadata: openAIMetadata(&fantasyopenai.WebSearchAction{
				Type:  "search",
				Query: "coder agents",
			}),
			want: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := chatopenai.WebSearchResultJSON(tt.metadata)
			require.True(t, ok)
			require.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestWebSearchResultJSONWithoutOpenAIAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata fantasy.ProviderMetadata
	}{
		{name: "NilMetadata"},
		{
			name: "NoAction",
			metadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: &fantasyopenai.WebSearchCallMetadata{ItemID: "ws_1"},
			},
		},
		{
			name: "AnthropicMetadata",
			metadata: fantasy.ProviderMetadata{
				fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := chatopenai.WebSearchResultJSON(tt.metadata)
			require.False(t, ok)
			require.Nil(t, got)
		})
	}
}
