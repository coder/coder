import { cn } from "cn";
import { InfoIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
	TOOLTIP_DELAY_DURATION,
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export type InfoTooltipType = "info" | "warning";

type InfoTooltipSize = "small" | "medium";

interface InfoTooltipProps {
	type?: InfoTooltipType;
	size?: InfoTooltipSize;
	children: ReactNode;
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
	children,
	type = "info",
	size = "medium",
}) => {
	const Icon = typeIcon[type];

	return (
		<TooltipProvider delayDuration={TOOLTIP_DELAY_DURATION}>
			<Tooltip>
				<TooltipTrigger
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
				</TooltipTrigger>
				<TooltipContent side="right" align="center" className="max-w-xs">
					{children}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
