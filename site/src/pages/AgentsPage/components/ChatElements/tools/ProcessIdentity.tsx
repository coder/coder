import { cn } from "cn";
import type React from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const PROCESS_ID_LENGTH = 8;

/**
 * Plain-text identity for a tracked process, rendered on the launch row and
 * on every later check row so the same token links them across arbitrary
 * transcript distance without mutating earlier rows.
 */
export const ProcessIdentity: React.FC<{
	processId: string;
	className?: string;
}> = ({ processId, className }) => {
	const shortId = processId.slice(0, PROCESS_ID_LENGTH);
	if (!shortId) {
		return null;
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					aria-label={`Process ${processId}`}
					role="img"
					className={cn(
						"hidden shrink-0 font-mono text-2xs leading-5 text-content-secondary sm:inline-flex",
						className,
					)}
				>
					{shortId}
				</span>
			</TooltipTrigger>
			<TooltipContent>Process {processId}</TooltipContent>
		</Tooltip>
	);
};
