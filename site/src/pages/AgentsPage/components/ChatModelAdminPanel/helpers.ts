import {
	getDefaultProviderBaseURL,
	normalizeProvider,
} from "#/modules/aiModels/helpers";

export function getProviderBaseURLPlaceholder(provider: string): string {
	switch (normalizeProvider(provider)) {
		case "azure":
			return "https://<resource-name>.openai.azure.com";
		case "bedrock":
			return "https://bedrock-runtime.<region>.amazonaws.com";
		case "openai-compat":
			return "https://api.example.com/v1";
		default:
			return getDefaultProviderBaseURL(provider) || "https://api.example.com";
	}
}
