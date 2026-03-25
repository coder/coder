import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ToolStatus } from "./utils";

/**
 * Simple inline rendering for `read_template` tool calls.
 * Shows "Read template <name>" with no expandable content.
 */
export const ReadTemplateTool: React.FC<{
	templateName: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ templateName, status, isError, errorMessage }) => {
	const isRunning = status === "running";

	const label = isRunning
		? "Reading template…"
		: templateName
			? `Read template ${templateName}`
			: "Read template";

	return (
		<div className="flex items-center gap-1.5">
			<span className={cn("text-sm", "text-content-secondary")}>{label}</span>
			{isError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
					</TooltipTrigger>
					<TooltipContent>
						{errorMessage || "Failed to read template"}
					</TooltipContent>
				</Tooltip>
			)}
			{isRunning && (
				<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
			)}
		</div>
	);
};
