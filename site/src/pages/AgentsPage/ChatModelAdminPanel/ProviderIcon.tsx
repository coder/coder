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

type ProviderIconProps = {
	provider: string;
	className?: string;
};

export const ProviderIcon: FC<ProviderIconProps> = ({
	provider,
	className,
}) => {
	const normalized = normalizeProvider(provider);
	const iconPath = providerIconMap[normalized];
	if (iconPath) {
		return (
			<div
				className={cn(
					"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
					className,
				)}
			>
				<ExternalImage
					src={iconPath}
					alt={`${formatProviderLabel(provider)} logo`}
					className="h-3/5 w-3/5"
				/>
			</div>
		);
	}
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			<ServerIcon className="h-3/5 w-3/5 text-content-secondary" />
		</div>
	);
};
