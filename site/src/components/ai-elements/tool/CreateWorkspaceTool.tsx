import { useTheme } from "@emotion/react";
import { File as FileViewer } from "@pierre/diffs/react";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronDownIcon, CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { useState } from "react";
import { cn } from "utils/cn";
import {
	DIFFS_FONT_STYLE,
	getFileViewerOptionsMinimal,
	type ToolStatus,
} from "./utils";

/**
 * Collapsed-by-default rendering for `create_workspace` tool calls.
 *
 * Once complete the result becomes a JSON object with workspace metadata.
 * Shows "Creating workspace…" while running, and "Created <name>" when
 * complete, expandable to reveal the full result JSON.
 */
export const CreateWorkspaceTool: React.FC<{
	workspaceName: string;
	resultJson: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({
	workspaceName,
	resultJson,
	status,
	isError,
	errorMessage,
}) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const [expanded, setExpanded] = useState(false);
	const isRunning = status === "running";
	const hasContent = resultJson.length > 0;

	const label = isRunning
		? "Creating workspace…"
		: workspaceName
			? `Created ${workspaceName}`
			: "Created workspace";

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				aria-expanded={expanded}
				onClick={() => hasContent && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && hasContent) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2",
					hasContent && "cursor-pointer",
				)}
			>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to create workspace"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
				)}
				{hasContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{/* Expandable JSON result once workspace creation completes. */}
			{expanded && hasContent && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileViewer
						file={{
							name: "result.json",
							contents: resultJson,
						}}
						options={getFileViewerOptionsMinimal(isDark)}
						style={DIFFS_FONT_STYLE}
					/>
				</ScrollArea>
			)}
		</div>
	);
};
