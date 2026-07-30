package chatopenai_test

import (
	"reflect"
	"strings"
	"testing"

	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

// openAIOptionTransportSupport records, per config field, which OpenAI wire
// formats honor it. It mirrors the table in ARCHITECTURE.md under "OpenAI
// transport selection". TestProviderOptionsTransportParity fails when a field
// is honored on one transport and silently ignored on the other without being
// recorded here, and when a new field is added without a row.
var openAIOptionTransportSupport = map[string]struct {
	responses       bool
	chatCompletions bool
}{
	"include":               {responses: true},
	"instructions":          {responses: true},
	"logit_bias":            {chatCompletions: true},
	"log_probs":             {responses: true, chatCompletions: true},
	"top_log_probs":         {responses: true, chatCompletions: true},
	"max_tool_calls":        {responses: true},
	"parallel_tool_calls":   {responses: true, chatCompletions: true},
	"user":                  {responses: true, chatCompletions: true},
	"reasoning_summary":     {responses: true},
	"max_completion_tokens": {chatCompletions: true},
	"text_verbosity":        {responses: true, chatCompletions: true},
	"prediction":            {chatCompletions: true},
	"store":                 {responses: true, chatCompletions: true},
	"metadata":              {responses: true, chatCompletions: true},
	"prompt_cache_key":      {responses: true, chatCompletions: true},
	"safety_identifier":     {responses: true, chatCompletions: true},
	"service_tier":          {responses: true, chatCompletions: true},
	"structured_outputs":    {chatCompletions: true},
	"strict_json_schema":    {responses: true},
	// Web search fields configure tool wiring, not per-request provider
	// options.
	"web_search_enabled":  {},
	"search_context_size": {},
	"allowed_domains":     {},
}

// Each field carries a valid non-zero value that changes the converted output
// on every transport that honors the field. service_tier uses a tier fantasy
// declares no constant for, so the conversion cannot pass by matching
// constants only.
var sampledOpenAIOptions = codersdk.ChatModelOpenAIProviderOptions{
	Include:             []string{string(fantasyopenai.IncludeFileSearchCallResults)},
	Instructions:        ptr("instructions"),
	LogitBias:           map[string]int64{"50256": -10},
	LogProbs:            ptr(true),
	TopLogProbs:         ptr(int64(3)),
	MaxToolCalls:        ptr(int64(8)),
	ParallelToolCalls:   ptr(true),
	User:                ptr("user-1"),
	ReasoningSummary:    ptr("auto"),
	MaxCompletionTokens: ptr(int64(4096)),
	TextVerbosity:       ptr("high"),
	Prediction:          map[string]any{"type": "content"},
	Store:               ptr(false),
	Metadata:            map[string]any{"scope": "unit"},
	PromptCacheKey:      ptr("cache-key"),
	SafetyIdentifier:    ptr("safety-id"),
	ServiceTier:         ptr("default"),
	StructuredOutputs:   ptr(true),
	StrictJSONSchema:    ptr(true),
	WebSearchEnabled:    ptr(true),
	SearchContextSize:   ptr("low"),
	AllowedDomains:      []string{"example.com"},
}

func TestProviderOptionsTransportParity(t *testing.T) {
	t.Parallel()

	optionsType := reflect.TypeOf(codersdk.ChatModelOpenAIProviderOptions{})
	require.Len(t, openAIOptionTransportSupport, optionsType.NumField())

	for i := 0; i < optionsType.NumField(); i++ {
		field := optionsType.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		want, ok := openAIOptionTransportSupport[name]
		require.Truef(t, ok, "field %s is missing from openAIOptionTransportSupport", name)

		sample := reflect.ValueOf(sampledOpenAIOptions).Field(i)
		require.Falsef(t, sample.IsZero(), "field %s needs a sample value in sampledOpenAIOptions", name)

		options := &codersdk.ChatModelOpenAIProviderOptions{}
		reflect.ValueOf(options).Elem().Field(i).Set(sample)

		require.Equalf(t, want.responses,
			optionChangesConvertedOutput(t, ptr(true), options),
			"field %s on the Responses transport", name)
		require.Equalf(t, want.chatCompletions,
			optionChangesConvertedOutput(t, ptr(false), options),
			"field %s on the Chat Completions transport", name)
	}
}

// optionChangesConvertedOutput reports whether converting options produces a
// different result than converting an empty config on the same transport.
func optionChangesConvertedOutput(
	t *testing.T,
	responsesOverride *bool,
	options *codersdk.ChatModelOpenAIProviderOptions,
) bool {
	t.Helper()
	model := fakeLanguageModel{provider: fantasyopenai.Name, model: "gpt-4.1"}
	baseline := chatopenai.ProviderOptionsFromChatConfig(
		model, &codersdk.ChatModelOpenAIProviderOptions{}, responsesOverride,
	)
	converted := chatopenai.ProviderOptionsFromChatConfig(model, options, responsesOverride)
	return !reflect.DeepEqual(baseline, converted)
}
