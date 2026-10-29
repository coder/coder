import type React from "react";
import { ToolCall } from "./ToolCall";
import { ToolResultImage } from "./ToolResultImage";
import type { ToolStatus } from "./utils";

const computerActionLabel = (action: string, isRunning: boolean): string => {
	if (action === "" || action === "screenshot") {
		return isRunning ? "Taking screenshot…" : "Screenshot";
	}
	const words = action.replaceAll("_", " ");
	const label = words.charAt(0).toUpperCase() + words.slice(1);
	return isRunning ? `${label}…` : label;
};

/**
 * Renders a computer use tool call: the screenshot it returned, or the
 * text output of a desktop action such as a click. When the result
 * contains base64 image data, the actual image is displayed instead of
 * raw JSON. Clicking the image opens it in an in-app lightbox overlay
 * rather than a new tab so that it works correctly in PWA / standalone
 * mode on iOS.
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
	const hasImage = imageData.length > 0;
	const hasText = text.length > 0;
	const hasContent = hasImage || hasText;
	const isScreenshot = action === "" || action === "screenshot";

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
			<ToolCall.Header
				iconName="computer"
				label={computerActionLabel(action, isRunning)}
			/>
			<ToolCall.Content>
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
