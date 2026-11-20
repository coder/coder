import type React from "react";
import { useContext } from "react";
import { Button } from "#/components/Button/Button";
import {
	PreviewPanelContext,
	parseOpenPreviewResult,
} from "./PreviewPanelContext";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";

/**
 * Rendering for `open_preview` tool calls, with an action that opens the
 * preview's right panel tab when the chat can show it.
 */
export const OpenPreviewTool: React.FC<{
	args: unknown;
	result: unknown;
	status: ToolStatus;
	isError: boolean;
}> = ({ args, result, status, isError }) => {
	const { getOpenAction } = useContext(PreviewPanelContext);
	const failed = status !== "running" && (isError || status === "error");
	const preview = failed ? undefined : parseOpenPreviewResult(result);
	const open = preview ? getOpenAction(preview) : undefined;
	const title =
		preview?.title || asString(parseArgs(args)?.title).trim() || "preview";

	const label =
		status === "running"
			? `Preparing ${title}…`
			: failed
				? `Failed to open ${title}`
				: title;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={
				asString(asRecord(result)?.error) || "Failed to open preview"
			}
			hasContent={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.Header iconName="open_preview" label={label} />
				{open && (
					<ToolCall.HeaderActions>
						<Button size="xs" variant="outline" onClick={open}>
							Open preview
						</Button>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
		</ToolCall.Root>
	);
};
