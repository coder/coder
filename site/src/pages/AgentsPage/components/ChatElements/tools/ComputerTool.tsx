import type React from "react";
import { ToolCall } from "./ToolCall";
import { ToolResultImage } from "./ToolResultImage";
import { humanizeMCPToolName, type ToolStatus } from "./utils";

type ComputerToolProps = {
	action: string;
	imageData: string;
	mimeType: string;
	text: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
};

/**
 * Shows the screenshot or the text output of a computer use call.
 * Clicking the image opens an in-app lightbox rather than a new tab,
 * so it works in PWA / standalone mode on iOS.
 */
export const ComputerTool: React.FC<ComputerToolProps> = ({
	action,
	imageData,
	mimeType,
	text,
	status,
	isError,
	errorMessage,
}) => {
	const isRunning = status === "running";
	const isScreenshot = action === "" || action === "screenshot";
	const errorDetail = isError ? errorMessage?.trim() : undefined;
	const hasImage = imageData.length > 0;
	const hasText = text.length > 0;
	const hasContent = hasImage || hasText || Boolean(errorDetail);

	let label = isRunning ? "Taking screenshot…" : "Screenshot";
	if (isRunning && !isScreenshot) {
		label = "Using the computer…";
	} else if (action === "key") {
		label = "Press key";
	} else if (!isScreenshot) {
		label = humanizeMCPToolName("", action);
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
					<ToolCall.ErrorDetail>{errorDetail}</ToolCall.ErrorDetail>
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
