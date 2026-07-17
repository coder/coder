import { type FC, useState } from "react";
import type { MergedTool } from "../../ChatConversation/types";
import { getReadFileToolData, ReadFileTool } from "./ReadFileTool";
import { ToolCall } from "./ToolCall";

type ReadFileItem = {
	id: string;
	path: string;
	content: string;
	status: MergedTool["status"];
	isError: boolean;
	errorMessage?: string;
};

const getReadFileItem = (tool: MergedTool): ReadFileItem => ({
	id: tool.id,
	status: tool.status,
	...getReadFileToolData(tool),
});

export const ReadFilesTool: FC<{
	tools: readonly MergedTool[];
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
}> = ({ tools, expanded, onExpandedChange }) => {
	const [expandedFileIDs, setExpandedFileIDs] = useState<ReadonlySet<string>>(
		new Set(),
	);
	const items = tools.map(getReadFileItem);
	const isRunning = tools.some((tool) => tool.status === "running");
	const isError = tools.some((tool) => tool.isError);
	const hasContent = items.length > 0;
	const label = isRunning
		? `Reading ${tools.length} files…`
		: `Read ${tools.length} files`;
	const errorMessage = items.find((item) => item.errorMessage)?.errorMessage;

	return (
		<div data-tool-call="">
			<ToolCall.Root
				className="w-full"
				status={isRunning ? "running" : isError ? "error" : "completed"}
				isError={isError}
				errorMessage={errorMessage || "Failed to read one or more files"}
				hasContent={hasContent}
				expanded={expanded}
				onExpandedChange={onExpandedChange}
			>
				<ToolCall.Header iconName="read_file" label={label} />
				<ToolCall.Content>
					<div className="space-y-1 py-0.5 pl-3">
						{items.map((item) => (
							<div key={item.id}>
								<ReadFileTool
									path={item.path}
									content={item.content}
									status={item.status}
									isError={item.isError}
									errorMessage={item.errorMessage}
									expanded={expandedFileIDs.has(item.id)}
									onExpandedChange={(nextExpanded) => {
										setExpandedFileIDs((previous) => {
											const next = new Set(previous);
											if (nextExpanded) {
												next.add(item.id);
											} else {
												next.delete(item.id);
											}
											return next;
										});
									}}
								/>
							</div>
						))}
					</div>
				</ToolCall.Content>
			</ToolCall.Root>
		</div>
	);
};
