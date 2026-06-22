import type React from "react";
import { useState } from "react";
import { ImageLightbox } from "../../ImageLightbox";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

/**
 * Renders screenshots returned by Anthropic's computer use tool.
 * When the result contains base64 image data, the actual image is
 * displayed instead of raw JSON. Clicking the image opens it in an
 * in-app lightbox overlay rather than a new tab so that it works
 * correctly in PWA / standalone mode on iOS.
 */
export const ComputerTool: React.FC<{
	imageData: string;
	mimeType: string;
	text: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ imageData, mimeType, text, status, isError, errorMessage }) => {
	const [showLightbox, setShowLightbox] = useState(false);
	const isRunning = status === "running";
	const hasImage = imageData.length > 0;
	const hasText = text.length > 0;
	const hasContent = hasImage || hasText;
	const imageSrc = hasImage ? `data:${mimeType};base64,${imageData}` : "";

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to take screenshot"}
			hasContent={hasContent}
			defaultExpanded={hasImage}
		>
			<ToolCall.Header
				iconName="computer"
				label={isRunning ? "Taking screenshot…" : "Screenshot"}
			/>
			<ToolCall.Content>
				{hasImage ? (
					<>
						<div className="mt-1.5 overflow-hidden rounded-md border border-solid border-border-default">
							<button
								type="button"
								className="cursor-pointer bg-transparent p-0 border-none"
								onClick={() => setShowLightbox(true)}
							>
								<img
									src={imageSrc}
									alt="Screenshot from computer tool"
									className="max-h-96 w-auto object-contain"
								/>
							</button>
						</div>
						{showLightbox && (
							<ImageLightbox
								src={imageSrc}
								onClose={() => setShowLightbox(false)}
							/>
						)}
					</>
				) : hasText ? (
					<div className="mt-1.5 rounded-md border border-solid border-border-default px-3 py-2">
						<pre className="whitespace-pre-wrap text-xs text-content-secondary">
							{text}
						</pre>
					</div>
				) : null}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
