import type React from "react";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

export const ReadSkillTool: React.FC<{
	label: string;
	body: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ label, body, status, isError, errorMessage }) => {
	const hasContent = body.length > 0;
	const isRunning = status === "running";

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to read skill"}
			hasContent={hasContent}
		>
			<ToolCall.Header
				iconName="read_skill"
				label={isRunning ? `Reading ${label}…` : `Read ${label}`}
			/>
			<ToolCall.Content>
				{body && (
					<ToolCall.Scrollable
						className="mt-1.5 rounded-md border border-solid border-border"
						viewportClassName="max-h-64"
						ariaLabel="Skill contents"
					>
						<div className="px-3 py-2">
							<Response>{body}</Response>
						</div>
					</ToolCall.Scrollable>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
