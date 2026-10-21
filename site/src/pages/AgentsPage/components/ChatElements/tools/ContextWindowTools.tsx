import type React from "react";
import { MarkdownToolRow } from "./MarkdownToolRow";
import { ToolCall } from "./ToolCall";
import type { ToolStatus } from "./utils";

type ToolRowStatusProps = {
	status: ToolStatus;
	isError: boolean;
	errorMessage: string | undefined;
};

// A Map because reasons come from persisted tool rows, so inherited object
// keys must not match.
const rolloverReasonLabels = new Map([
	["model", "Requested by the agent"],
	["limit", "Usable limit reached"],
	["overflow", "Provider context overflow"],
]);

const SecondaryLabel: React.FC<{ children: React.ReactNode }> = ({
	children,
}) => (
	<ToolCall.Label className="text-content-secondary/60">
		{children}
	</ToolCall.Label>
);

/**
 * Boundary row for a token budget rollover: a fresh context window that
 * starts from the latest checkpoint, when one was saved, instead of a
 * summary.
 */
export const ContextRolloverTool: React.FC<
	ToolRowStatusProps & { reason: string; checkpoint: string }
> = ({ reason, checkpoint, ...rowStatus }) => {
	const reasonLabel = rolloverReasonLabels.get(reason);
	return (
		<MarkdownToolRow
			{...rowStatus}
			iconName="chat_context_rollover"
			label="Context window rollover"
			secondaryLabel={
				reasonLabel ? <SecondaryLabel>{reasonLabel}</SecondaryLabel> : null
			}
			bodyLabel="Rollover checkpoint"
			body={checkpoint}
		/>
	);
};

/**
 * The prompt chatd sent when the window reached the handoff target, or the
 * final prompt before chatd forces a rollover.
 */
export const ContextHandoffTool: React.FC<
	ToolRowStatusProps & { stage: string; prompt: string }
> = ({ stage, prompt, ...rowStatus }) => (
	<MarkdownToolRow
		{...rowStatus}
		iconName="chat_context_handoff"
		label={
			stage === "final"
				? "Context window ending: final handoff"
				: "Context handoff requested"
		}
		bodyLabel="Handoff prompt"
		body={prompt}
	/>
);
