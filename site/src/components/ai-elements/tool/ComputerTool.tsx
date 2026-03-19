import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";

/**
 * Renders screenshots returned by Anthropic's computer use tool.
 * When the result contains base64 image data, the actual image is
 * displayed instead of raw JSON. The image is clickable and opens
 * in a new tab at full resolution.
 */
export const ComputerTool: React.FC<{
	imageData: string;
	mimeType: string;
	text: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ imageData, mimeType, text, status, isError, errorMessage }) => {
	const isRunning = status === "running";
	const hasImage = imageData.length > 0;
	const hasText = text.length > 0;
	const hasContent = hasImage || hasText;

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			defaultExpanded={hasImage}
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{isRunning ? "Taking screenshot…" : "Screenshot"}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to take screenshot"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			{hasImage ? (
				<div className="mt-1.5 overflow-hidden rounded-md border border-solid border-border-default">
					<a
						href={`data:${mimeType};base64,${imageData}`}
						target="_blank"
						rel="noopener noreferrer"
					>
						<img
							src={`data:${mimeType};base64,${imageData}`}
							alt="Screenshot from computer tool"
							className="max-h-96 w-auto object-contain"
						/>
					</a>
				</div>
			) : hasText ? (
				<div className="mt-1.5 rounded-md border border-solid border-border-default px-3 py-2">
					<pre className="whitespace-pre-wrap text-xs text-content-secondary">
						{text}
					</pre>
				</div>
			) : null}
		</ToolCollapsible>
	);
};
