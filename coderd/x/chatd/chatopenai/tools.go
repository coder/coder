package chatopenai

import (
	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk"
)

// WebSearchTool returns the OpenAI provider-native web search tool when
// enabled by the model provider options.
func WebSearchTool(options *codersdk.ChatModelOpenAIProviderOptions) (fantasy.Tool, bool) {
	if options == nil || options.WebSearchEnabled == nil || !*options.WebSearchEnabled {
		return nil, false
	}

	args := map[string]any{}
	if options.SearchContextSize != nil && *options.SearchContextSize != "" {
		args["search_context_size"] = *options.SearchContextSize
	}
	if len(options.AllowedDomains) > 0 {
		args["allowed_domains"] = options.AllowedDomains
	}

	return fantasy.ProviderDefinedTool{
		ID:   "web_search",
		Name: "web_search",
		Args: args,
	}, true
}
