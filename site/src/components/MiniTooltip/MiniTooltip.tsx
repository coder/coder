import {
	Tooltip,
	TooltipArrow,
	TooltipContent,
	type TooltipContentProps,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useState, type FC, type ReactNode } from "react";
import { cn } from "utils/cn";

type MiniTooltipProps = Omit<TooltipContentProps, 'title'> & {
	title: ReactNode;
	arrow?: boolean;
	open?: boolean;
};

const MiniTooltip: FC<MiniTooltipProps> = (props) => {
	const { title, children, arrow, open, ...contentProps } = props;
	const [isOpen, setIsOpen] = useState(false)

	return (
		<TooltipProvider>
			<Tooltip delayDuration={0} open={isOpen} onOpenChange={setIsOpen}>
				<TooltipTrigger asChild aria-label={typeof title === 'string' ? title : undefined}>
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
