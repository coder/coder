package chatprovider

import (
	"cmp"
	"context"
	"sort"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"golang.org/x/xerrors"
	"google.golang.org/genai"
)

// ListProviderModels lists the models available from the given
// provider. It uses the same SDK clients as the rest of the system
// so that auth mechanisms, base URLs, and header conventions stay
// consistent. The returned model IDs can be used directly when
// creating a ChatModelConfig.
//
// TODO: In the future, consider exposing richer model metadata
// (display names, context windows, capabilities) from the provider
// APIs to further streamline model configuration.
func ListProviderModels(ctx context.Context, provider, apiKey, baseURL string) ([]string, error) {
	provider = NormalizeProvider(provider)
	if provider == "" {
		return nil, xerrors.New("unsupported provider")
	}

	switch provider {
	case fantasyopenai.Name:
		return listOpenAIModels(ctx, apiKey, cmp.Or(baseURL, fantasyopenai.DefaultURL))
	case fantasyazure.Name:
		if baseURL == "" {
			return nil, xerrors.New("base URL is required for Azure OpenAI")
		}
		// Azure uses the "api-key" header instead of the standard
		// Authorization header for authentication.
		return listOpenAIModels(ctx, apiKey, baseURL,
			openaioption.WithHeader("api-key", apiKey))
	case fantasyopenaicompat.Name:
		if baseURL == "" {
			return nil, xerrors.New("base URL is required for OpenAI-compatible providers")
		}
		return listOpenAIModels(ctx, apiKey, baseURL)
	case fantasyopenrouter.Name:
		return listOpenAIModels(ctx, apiKey, cmp.Or(baseURL, fantasyopenrouter.DefaultURL))
	case fantasyvercel.Name:
		return listOpenAIModels(ctx, apiKey, cmp.Or(baseURL, fantasyvercel.DefaultURL))
	case fantasyanthropic.Name:
		return listAnthropicModels(ctx, apiKey, cmp.Or(baseURL, fantasyanthropic.DefaultURL))
	case fantasybedrock.Name:
		// Bedrock wraps the Anthropic SDK with WithBedrock(). Listing
		// models via the Anthropic SDK against Bedrock may not work
		// the same way. For now, return an empty list — the provider
		// construction itself validates credentials.
		return nil, nil
	case fantasygoogle.Name:
		return listGoogleModels(ctx, apiKey, baseURL)
	default:
		return nil, xerrors.Errorf("unsupported provider: %s", provider)
	}
}

// listOpenAIModels lists models from any OpenAI-compatible endpoint.
// Extra request options (e.g. Azure auth headers) can be appended
// via extraOpts.
func listOpenAIModels(ctx context.Context, apiKey, baseURL string, extraOpts ...openaioption.RequestOption) ([]string, error) {
	opts := []openaioption.RequestOption{
		openaioption.WithAPIKey(apiKey),
		openaioption.WithBaseURL(baseURL),
	}
	opts = append(opts, extraOpts...)

	client := openai.NewClient(opts...)
	pager := client.Models.ListAutoPaging(ctx)

	var ids []string
	for pager.Next() {
		ids = append(ids, pager.Current().ID)
	}
	if err := pager.Err(); err != nil {
		return nil, xerrors.Errorf("list openai models: %w", err)
	}

	sort.Strings(ids)
	return ids, nil
}

// listAnthropicModels lists models from the Anthropic API.
func listAnthropicModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	client := anthropic.NewClient(
		anthropicoption.WithAPIKey(apiKey),
		anthropicoption.WithBaseURL(baseURL),
	)

	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})

	var ids []string
	for pager.Next() {
		ids = append(ids, pager.Current().ID)
	}
	if err := pager.Err(); err != nil {
		return nil, xerrors.Errorf("list anthropic models: %w", err)
	}

	sort.Strings(ids)
	return ids, nil
}

// listGoogleModels lists models from the Google Generative AI API.
func listGoogleModels(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	cc := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}
	if baseURL != "" {
		cc.HTTPOptions = genai.HTTPOptions{
			BaseURL: baseURL,
		}
	}

	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, xerrors.Errorf("create google genai client: %w", err)
	}

	var ids []string
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, xerrors.Errorf("list google models: %w", err)
		}
		ids = append(ids, model.Name)
	}

	sort.Strings(ids)
	return ids, nil
}
