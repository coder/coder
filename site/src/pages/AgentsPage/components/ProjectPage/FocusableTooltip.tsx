import { cn } from "cn";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type FocusableTooltipProps = {
	readonly tooltip: React.ReactNode;
	readonly className?: string;
	readonly tooltipClassName?: string;
	readonly children: React.ReactNode;
};

/**
 * Text with a tooltip. The text is a button, so keyboard users can focus it
 * to read the tooltip.
 */
export const FocusableTooltip: React.FC<FocusableTooltipProps> = ({
	tooltip,
	className,
	tooltipClassName,
	children,
}) => (
	<Tooltip>
		<TooltipTrigger
			type="button"
			className={cn(
				"cursor-default rounded-sm bg-transparent p-0",
				"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				className,
			)}
		>
			{children}
		</TooltipTrigger>
		<TooltipContent className={tooltipClassName}>{tooltip}</TooltipContent>
	</Tooltip>
);
