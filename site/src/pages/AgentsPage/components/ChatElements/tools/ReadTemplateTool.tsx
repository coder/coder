import type React from "react";
import { ToolCall } from "./ToolCall";
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
		<ToolCall.Root
			status={status}
			isError={isError}
			errorMessage={errorMessage || "Failed to read template"}
			hasContent={false}
		>
			<ToolCall.Header iconName="read_template" label={label} />
		</ToolCall.Root>
	);
};
