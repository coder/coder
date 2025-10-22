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
	...props
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<SelectTrigger
						{...props}
						className={cn([
							className,
							`w-auto border-0 bg-surface-secondary text-sm text-content-primary gap-2 px-3
							[&_svg]:text-inherit cursor-pointer hover:bg-surface-quaternary rounded-full
							h-8 data-[state=open]:bg-surface-tertiary`,
						])}
					/>
				</TooltipTrigger>
				<TooltipContent>{tooltip}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
