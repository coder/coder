package chatopenai

import (
	"encoding/json"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"

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

// WebSearchResult is the tool result persisted for a provider-executed
// OpenAI web_search call so the chat UI can show which URLs the search
// consulted. The provider only reports them in response metadata, which API
// responses strip. The queries are in the call input.
type WebSearchResult struct {
	// Sources are the URLs the search consulted. They are distinct
	// from the url_citation annotations that become source parts.
	Sources []WebSearchSource `json:"sources,omitempty"`
}

// WebSearchSource is one URL a web search consulted.
type WebSearchSource struct {
	URL string `json:"url"`
}

// WebSearchResultJSON builds the WebSearchResult JSON for a web_search
// tool result from the OpenAI Responses call metadata. It reports false
// when the metadata carries no OpenAI web search action.
func WebSearchResultJSON(metadata fantasy.ProviderMetadata) (json.RawMessage, bool) {
	meta, ok := metadata[fantasyopenai.Name].(*fantasyopenai.WebSearchCallMetadata)
	if !ok || meta == nil || meta.Action == nil {
		return nil, false
	}

	var result WebSearchResult
	for _, source := range meta.Action.Sources {
		if source.URL != "" {
			result.Sources = append(result.Sources, WebSearchSource{URL: source.URL})
		}
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, false
	}
	return encoded, true
}
