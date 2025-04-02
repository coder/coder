import type { FC } from "react";
import {
	Tooltip,
	TooltipProvider,
	TooltipTrigger,
	TooltipContent,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";

export const LastConnectionHead: FC = () => {
	return (
		<span className="flex items-center gap-1 whitespace-nowrap text-xs font-medium text-content-secondary">
			Last connection
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<span className="flex items-center">
							<span className="sr-only">More info</span>
							<InfoIcon
								tabIndex={0}
								className="cursor-pointer size-icon-xs p-0.5"
							/>
						</span>
					</TooltipTrigger>
					<TooltipContent className="max-w-xs">
						Last time the provisioner connected to the control plane
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		</span>
	);
};
