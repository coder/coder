import { Building2Icon } from "lucide-react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type ProviderIconProps = {
	provider: string;
	icon?: string;
	className?: string;
};

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
		case "copilot":
			return "/icon/github-copilot.svg";
		case "google":
			return "/icon/google.svg";
		case "vercel":
			return "/icon/vercel.svg";
		default:
			return undefined;
	}
};

const getProviderName = (provider: string): string => {
	switch (provider) {
		case "openai":
			return "OpenAI";
		case "anthropic":
			return "Anthropic";
		case "bedrock":
			return "AWS Bedrock";
		case "azure":
			return "Azure OpenAI";
		case "copilot":
			return "GitHub Copilot";
		case "google":
			return "Google";
		case "openai-compat":
			return "OpenAI-compatible";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel";
		default:
			return provider || "Unknown provider";
	}
};

export const ProviderIcon: React.FC<ProviderIconProps> = ({
	provider,
	icon,
	className = "size-icon-sm",
}) => {
	const iconSrc = icon || getProviderIcon(provider);
	const name = getProviderName(provider);
	if (iconSrc === undefined) {
		return (
			<Building2Icon
				className={`${className} flex-shrink-0`}
				aria-label={name}
			/>
		);
	}
	return <ExternalImage src={iconSrc} alt={name} className={className} />;
};
