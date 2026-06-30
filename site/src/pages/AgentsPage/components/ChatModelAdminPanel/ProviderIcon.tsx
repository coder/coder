import { ServerIcon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { cn } from "#/utils/cn";
import { formatProviderLabel } from "../../utils/modelOptions";

const providerIconMap: Record<string, string> = {
	openai: "/icon/openai.svg",
	anthropic: "/icon/anthropic.svg",
	azure: "/icon/azure.svg",
	bedrock: "/icon/aws.svg",
	"claude-platform-aws": "/icon/aws.svg",
	google: "/icon/google.svg",
	gemini: "/icon/gemini.svg",
};

interface ProviderIconProps {
	provider: string;
	className?: string;
}

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
					className="size-3/5"
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
			<ServerIcon className="size-3/5 text-content-secondary" />
		</div>
	);
};
