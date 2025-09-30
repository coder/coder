import {
	Tooltip,
	TooltipArrow,
	TooltipContent,
	type TooltipContentProps,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import { cn } from "utils/cn";

type MiniTooltipProps = TooltipContentProps & {
	title: string;
	arrow?: boolean;
};

const MiniTooltip: FC<MiniTooltipProps> = (props) => {
	const { title, children, arrow, ...contentProps } = props;

	return (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild aria-label={title}>
					{children}
				</TooltipTrigger>
				<TooltipContent
					collisionPadding={16}
					side="bottom"
					{...contentProps}
					className={cn(
						"max-w-[300px] bg-surface-secondary border-surface-quaternary text-content-primary text-xs",
						contentProps.className,
					)}
				>
					{title}
					{arrow && <TooltipArrow className="fill-surface-quaternary" />}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

export default MiniTooltip;
