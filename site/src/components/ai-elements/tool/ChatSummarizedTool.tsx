import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import { Response } from "../response";
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";

/**
 * Collapsed-by-default rendering for `chat_summarized` tool calls.
 * Shows "Summarized" and reveals the summary only when expanded.
 */
export const ChatSummarizedTool: React.FC<{
	summary: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ summary, status, isError, errorMessage }) => {
	const hasSummary = summary.trim().length > 0;
	const isRunning = status === "running";

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasSummary}
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{isRunning ? "Summarizing…" : "Summarized"}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to summarize chat"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<ScrollArea
				className="mt-1.5 rounded-md border border-solid border-border-default"
				viewportClassName="max-h-64"
				scrollBarClassName="w-1.5"
			>
				<div className="px-3 py-2">
					<Response>{summary}</Response>
				</div>
			</ScrollArea>
		</ToolCollapsible>
	);
};
