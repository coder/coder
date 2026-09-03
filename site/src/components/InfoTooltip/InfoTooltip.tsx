import { InfoIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
	TOOLTIP_DELAY_DURATION,
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

type InfoTooltipType = "info" | "warning";

type InfoTooltipSize = "small" | "medium";

interface InfoTooltipProps {
	type?: InfoTooltipType;
	size?: InfoTooltipSize;
	title?: ReactNode;
	message: ReactNode;
}

const typeIcon: Record<InfoTooltipType, typeof InfoIcon> = {
	info: InfoIcon,
	warning: TriangleAlertIcon,
};

const typeIconColor: Record<InfoTooltipType, string> = {
	info: "text-content-secondary",
	warning: "text-content-warning",
};

const sizeClasses: Record<InfoTooltipSize, string> = {
	small: "[&_svg]:size-3",
	medium: "[&_svg]:size-4",
};

export const InfoTooltip: FC<InfoTooltipProps> = ({
	title,
	message,
	type = "info",
	size = "medium",
}) => {
	const Icon = typeIcon[type];

	return (
		<TooltipProvider delayDuration={TOOLTIP_DELAY_DURATION}>
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						aria-label="More info"
						className={cn(
							"flex items-center justify-center p-0",
							"border-0 border-none bg-transparent cursor-default",
							"opacity-75 hover:opacity-100 transition-opacity",
							sizeClasses[size],
							typeIconColor[type],
						)}
					>
						<Icon />
					</button>
				</TooltipTrigger>
				<TooltipContent side="right" align="center" className="max-w-xs">
					{title && (
						<p className="m-0 mb-1 font-semibold text-content-primary">
							{title}
						</p>
					)}
					<p className="m-0 text-content-secondary [&_a]:mt-2">{message}</p>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
