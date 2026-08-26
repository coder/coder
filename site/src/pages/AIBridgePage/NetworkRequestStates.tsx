import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

// Shared by the sessions list badges and the session detail summary card, which
// render the same two non-numeric states for a session's network requests but
// differ in how they present a live count.

export const NetworkMonitoringDisabled: FC = () => (
	<span className="inline-flex items-center gap-1 whitespace-nowrap text-content-secondary">
		Disabled
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<button
						type="button"
						aria-label="Why network request tracking is disabled"
						className="flex items-center justify-center border-0 bg-transparent p-0 text-content-secondary"
						onClick={(event) => event.stopPropagation()}
					>
						<InfoIcon className="size-3" />
					</button>
				</TooltipTrigger>
				<TooltipContent side="top" align="start" className="max-w-[320px]">
					Agent Firewall is off. Enable it in the workspace template to track
					network requests.
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	</span>
);

export const NetworkNoActivity: FC = () => (
	<span className="whitespace-nowrap text-content-secondary">No activity</span>
);
