import { Building2Icon } from "lucide-react";
import { useState } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { normalizeProvider } from "#/modules/aiModels/helpers";

type ProviderIconProps = {
	provider: string;
	icon?: string;
	className?: string;
	/** Defaults to decorative. Set it when no adjacent text names the provider. */
	alt?: string;
};

export const getProviderIcon = (provider: string): string | undefined => {
	switch (normalizeProvider(provider)) {
		case "openai":
			return "/icon/openai.svg";
		case "anthropic":
			return "/icon/anthropic.svg";
		case "bedrock":
			return "/icon/aws.svg";
		case "claude-platform-aws":
			return "/icon/aws.svg";
		case "azure":
			return "/icon/azure.svg";
		case "copilot":
			return "/icon/github-copilot.svg";
		case "google":
			return "/icon/google.svg";
		case "vercel":
			return "/icon/vercel.svg";
		case "gemini":
			return "/icon/gemini.svg";
		case "openrouter":
			return "/icon/openrouter.svg";
		case "opencode-go":
			return "/icon/opencode.svg";
		case "zai":
			return "/icon/zai.svg";
		default:
			return undefined;
	}
};

export const ProviderIcon: React.FC<ProviderIconProps> = ({
	provider,
	icon,
	className = "size-icon-sm",
	alt = "",
}) => {
	// Keyed by URL, so a new icon gets a fresh attempt.
	const [failedSrcs, setFailedSrcs] = useState<readonly string[]>([]);
	const iconSrc = [icon, getProviderIcon(provider)].find(
		(src) => src && !failedSrcs.includes(src),
	);
	if (iconSrc === undefined) {
		const fallbackClassName = `${className} shrink-0`;
		return alt ? (
			<Building2Icon
				role="img"
				aria-label={alt}
				className={fallbackClassName}
			/>
		) : (
			<Building2Icon aria-hidden="true" className={fallbackClassName} />
		);
	}
	return (
		<ExternalImage
			src={iconSrc}
			alt={alt}
			className={className}
			onError={() => setFailedSrcs((prev) => [...prev, iconSrc])}
		/>
	);
};
