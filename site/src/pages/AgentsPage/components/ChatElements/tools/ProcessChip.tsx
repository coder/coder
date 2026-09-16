import { cn } from "cn";
import type React from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const PROCESS_CHIP_ID_LENGTH = 8;

/**
 * Short, non-interactive identity chip for a tracked process. Rendered on the
 * launch row and on every later check row so the same token links them across
 * arbitrary transcript distance without mutating earlier rows.
 */
export const ProcessChip: React.FC<{
	processId: string;
	className?: string;
}> = ({ processId, className }) => {
	const shortId = processId.slice(0, PROCESS_CHIP_ID_LENGTH);
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
						"hidden shrink-0 rounded bg-surface-secondary px-1.5 py-0.5 font-mono text-2xs leading-none text-content-secondary sm:inline-flex",
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
