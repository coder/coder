import { CircleQuestionMarkIcon } from "lucide-react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type ProviderIconProps = {
	provider: string;
};

/**
 * Path to the provider's bundled icon, or `undefined` when we don't have a
 * dedicated icon for the type. Types listed in the "Add provider" dropdown
 * without a bundled icon fall back to a neutral question-mark glyph so the
 * row still reads as a real entry.
 */
export const getProviderIcon = (provider: string): string | undefined => {
	switch (provider) {
		case "openai":
			return "/icon/openai.svg";
		case "anthropic":
			return "/icon/anthropic.svg";
		case "bedrock":
			return "/icon/aws.svg";
		case "azure":
			return "/icon/azure.svg";
		case "google":
			return "/icon/google.svg";
		default:
			return undefined;
	}
};

/**
 * Human-friendly display name for a provider type. Falls back to the raw
 * provider string so we never render an empty label.
 */
const getProviderName = (provider: string): string => {
	switch (provider) {
		case "openai":
			return "OpenAI";
		case "anthropic":
			return "Anthropic";
		case "bedrock":
			return "AWS Bedrock";
		case "azure":
			return "Azure OpenAI Service";
		case "google":
			return "Google";
		case "openai-compat":
			return "OpenAI via bridge";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel AI Gateway";
		default:
			return provider || "Unknown provider";
	}
};

export const ProviderIcon: React.FC<ProviderIconProps> = ({ provider }) => {
	const iconSrc = getProviderIcon(provider);
	const name = getProviderName(provider);
	if (iconSrc === undefined) {
		return (
			<CircleQuestionMarkIcon
				className="size-icon-sm flex-shrink-0"
				aria-label={name}
			/>
		);
	}
	return <ExternalImage src={iconSrc} alt={name} className="size-icon-sm" />;
};
