import { LoaderIcon, MonitorIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
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
}> = ({ imageData, mimeType, text, status, isError }) => {
	const isRunning = status === "running";
	const hasImage = imageData.length > 0;

	return (
		<div className="w-full">
			<div className="flex items-center gap-2">
				<MonitorIcon
					className={cn(
						"h-4 w-4 shrink-0",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				/>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{isRunning ? "Taking screenshot…" : "Screenshot"}
				</span>
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
				)}
			</div>
			{hasImage ? (
				<div className="mt-1.5 ml-6 overflow-hidden rounded-md border border-solid border-border-default">
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
			) : text ? (
				<div className="mt-1.5 ml-6 rounded-md border border-solid border-border-default px-3 py-2">
					<pre className="whitespace-pre-wrap text-xs text-content-secondary">
						{text}
					</pre>
				</div>
			) : null}
		</div>
	);
};
