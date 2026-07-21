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

export const ProviderIcon: React.FC<ProviderIconProps> = ({
	provider,
	icon,
	className = "size-icon-sm",
}) => {
	const iconSrc = icon || getProviderIcon(provider);
	if (iconSrc === undefined) {
		// Lucide icons are aria-hidden by default when given no a11y prop.
		return <Building2Icon className={`${className} flex-shrink-0`} />;
	}
	// Decorative: callers render the provider name as visible text beside the
	// icon, so exposing it here too would double-announce it.
	return (
		<ExternalImage src={iconSrc} alt="" aria-hidden className={className} />
	);
};
