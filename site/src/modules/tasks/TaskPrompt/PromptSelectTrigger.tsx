import {
	SelectTrigger,
	type SelectTriggerProps,
} from "components/Select/Select";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { FC } from "react";
import { cn } from "utils/cn";

type PromptSelectTriggerProps = SelectTriggerProps & {
	tooltip: string;
};

export const PromptSelectTrigger: FC<PromptSelectTriggerProps> = ({
	className,
	tooltip,
	children,
	...props
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<SelectTrigger
						{...props}
						className={cn([
							`w-full md:w-auto max-w-full overflow-hidden border-0 bg-surface-secondary text-sm text-content-primary gap-2 px-4 md:px-3
							[&_svg]:text-inherit cursor-pointer hover:bg-surface-quaternary rounded-full
							h-10 md:h-8 data-[state=open]:bg-surface-tertiary`,
							className,
						])}
					>
						<span
							data-slot="value"
							className="overflow-hidden min-w-0 flex items-center gap-2"
						>
							{children}
						</span>
					</SelectTrigger>
				</TooltipTrigger>
				<TooltipContent>{tooltip}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
