import type React from "react";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

/**
 * A collapsible tool row whose expanded body renders `body` as Markdown in
 * a height-capped scroll region named by `bodyLabel`.
 */
export const MarkdownToolRow: React.FC<{
	iconName: string;
	label: string;
	secondaryLabel?: React.ReactNode;
	bodyLabel: string;
	body: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage: string | undefined;
}> = ({
	iconName,
	label,
	secondaryLabel,
	bodyLabel,
	body,
	status,
	isError,
	errorMessage,
}) => (
	<ToolCall.Root
		className="w-full"
		status={status}
		isError={isError}
		errorMessage={errorMessage}
		hasContent={body.trim().length > 0}
	>
		<ToolCall.Header
			iconName={iconName}
			label={label}
			secondaryLabel={secondaryLabel}
		/>
		<ToolCall.Content>
			<ToolCall.Scrollable
				className="mt-1.5 rounded-md border border-solid border-border"
				viewportClassName="max-h-64"
				ariaLabel={bodyLabel}
			>
				<div className="px-3 py-2">
					<Response>{body}</Response>
				</div>
			</ToolCall.Scrollable>
		</ToolCall.Content>
	</ToolCall.Root>
);
