import { cn } from "cn";
import { InfoIcon, TriangleAlertIcon } from "lucide-react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export type InfoTooltipType = "info" | "warning";

type InfoTooltipSize = "small" | "medium";

type InfoTooltipProps = {
	type?: InfoTooltipType;
	size?: InfoTooltipSize;
	ariaLabel?: string;
	children: React.ReactNode;
};

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

export const InfoTooltip: React.FC<InfoTooltipProps> = ({
	children,
	type = "info",
	size = "medium",
	ariaLabel = "More info",
}) => {
	const Icon = typeIcon[type];

	return (
		<Tooltip interactive>
			<TooltipTrigger
				type="button"
				aria-label={ariaLabel}
				className={cn(
					"flex items-center justify-center p-0",
					"border-0 border-none bg-transparent cursor-default",
					"opacity-75 hover:opacity-100 transition-opacity",
					"rounded-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
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
	);
};
