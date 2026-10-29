import type React from "react";
import { ToolCall } from "./ToolCall";
import { ToolResultImage } from "./ToolResultImage";
import { humanizeMCPToolName, type ToolStatus } from "./utils";

/**
 * Shows the screenshot or the text output of a computer use call.
 * Clicking the image opens an in-app lightbox rather than a new tab,
 * so it works in PWA / standalone mode on iOS.
 */
export const ComputerTool: React.FC<{
	action: string;
	imageData: string;
	mimeType: string;
	text: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ action, imageData, mimeType, text, status, isError, errorMessage }) => {
	const isRunning = status === "running";
	const isScreenshot = action === "" || action === "screenshot";
	const errorDetail = isError ? errorMessage?.trim() : undefined;
	const hasImage = imageData.length > 0;
	const hasText = text.length > 0;
	const hasContent = hasImage || hasText || Boolean(errorDetail);

	let label = isRunning ? "Taking screenshot…" : "Screenshot";
	if (!isScreenshot) {
		label = isRunning ? "Using the computer…" : humanizeMCPToolName("", action);
	}

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={
				errorMessage ||
				(isScreenshot ? "Failed to take screenshot" : "Computer action failed")
			}
			hasContent={hasContent}
			defaultExpanded={hasImage}
		>
			<ToolCall.Header iconName="computer" label={label} />
			<ToolCall.Content>
				{errorDetail && (
					<pre className="m-0 mt-1.5 whitespace-pre-wrap break-all border-0 bg-transparent p-0 font-mono text-xs leading-5 text-content-destructive">
						{errorDetail}
					</pre>
				)}
				{hasImage ? (
					<ToolResultImage
						data={imageData}
						mimeType={mimeType}
						alt="Screenshot from computer tool"
					/>
				) : hasText ? (
					<div className="mt-1.5 rounded-md border border-solid border-border px-3 py-2">
						<pre className="whitespace-pre-wrap text-xs text-content-secondary">
							{text}
						</pre>
					</div>
				) : null}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
