import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { TranscriptRow } from "../TranscriptRow";
import { ToolIcon } from "./ToolIcon";
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
		<TranscriptRow className="gap-2 text-content-secondary">
			<ToolIcon name="read_template" isError={isError} isRunning={isRunning} />
			<span className="text-[13px] leading-6">{label}</span>
			{isError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<TriangleAlertIcon className="size-3.5 shrink-0 text-current" />
					</TooltipTrigger>
					<TooltipContent>
						{errorMessage || "Failed to read template"}
					</TooltipContent>
				</Tooltip>
			)}
			{isRunning && (
				<LoaderIcon className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
			)}
		</TranscriptRow>
	);
};
