import { Building2Icon } from "lucide-react";
import type { FC } from "react";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { getProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { cn } from "#/utils/cn";

interface ProviderIconProps {
	provider: string;
	className?: string;
}

export const ProviderIcon: FC<ProviderIconProps> = ({
	provider,
	className,
}) => {
	const iconPath = getProviderIcon(normalizeProvider(provider));
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			{iconPath ? (
				<ExternalImage src={iconPath} alt="" className="size-3/5" />
			) : (
				<Building2Icon className="size-3/5 text-content-secondary" />
			)}
		</div>
	);
};
