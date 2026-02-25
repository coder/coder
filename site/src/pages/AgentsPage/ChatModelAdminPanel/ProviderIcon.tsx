import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";
import { formatProviderLabel } from "../modelOptions";
import { normalizeProvider } from "./helpers";

const providerIconMap: Record<string, string> = {
	openai: "/icon/openai.svg",
	anthropic: "/icon/claude.svg",
	azure: "/icon/azure.svg",
	bedrock: "/icon/aws.svg",
	google: "/icon/google.svg",
	gemini: "/icon/gemini.svg",
};

// Some provider SVGs (e.g. OpenAI) are pure black and need
// inversion in dark mode to remain visible.
const darkInvertProviders = new Set(["openai"]);

type ProviderIconProps = {
	provider: string;
	className?: string;
	active?: boolean;
};

export const ProviderIcon: FC<ProviderIconProps> = ({
	provider,
	className,
	active,
}) => {
	const normalized = normalizeProvider(provider);
	const iconPath = providerIconMap[normalized];
	if (iconPath) {
		return (
			<ExternalImage
				src={iconPath}
				alt={`${formatProviderLabel(provider)} logo`}
				className={cn(
					"shrink-0",
					!active && "grayscale opacity-50",
					darkInvertProviders.has(normalized) && "dark:invert",
					className,
				)}
			/>
		);
	}
	return (
		<ServerIcon
			className={cn(
				"shrink-0",
				active ? "text-content-primary" : "text-content-secondary",
				className,
			)}
		/>
	);
};
