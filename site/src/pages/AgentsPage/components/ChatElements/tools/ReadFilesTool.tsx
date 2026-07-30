import { type FC, useState } from "react";
import type { MergedTool } from "../../ChatConversation/types";
import { getReadFileToolData, ReadFileTool } from "./ReadFileTool";
import { ToolCall } from "./ToolCall";

type ReadFileItem = {
	path: string;
	content: string;
	status: MergedTool["status"];
	isError: boolean;
	errorMessage?: string;
};

const getReadFileItem = (tool: MergedTool): ReadFileItem => ({
	status: tool.status,
	...getReadFileToolData(tool),
});

export const ReadFilesTool: FC<{
	tools: readonly [MergedTool, ...MergedTool[]];
}> = ({ tools }) => {
	const [expandedFileIndexes, setExpandedFileIndexes] = useState<
		ReadonlySet<number>
	>(new Set());
	const items = tools.map(getReadFileItem);
	const isRunning = tools.some((tool) => tool.status === "running");
	const isError = tools.some((tool) => tool.isError);
	const label = isRunning
		? `Reading ${tools.length} files…`
		: `Read ${tools.length} files`;
	const errorMessage = items.find((item) => item.errorMessage)?.errorMessage;

	return (
		<div data-transcript-row="">
			<ToolCall.Root
				className="w-full"
				status={isRunning ? "running" : isError ? "error" : "completed"}
				isError={isError}
				errorMessage={errorMessage || "Failed to read one or more files"}
			>
				<ToolCall.Header iconName="read_file" label={label} />
				<ToolCall.Content>
					<div className="space-y-1 py-0.5 pl-3">
						{items.map((item, index) => (
							<div key={index}>
								<ReadFileTool
									path={item.path}
									content={item.content}
									status={item.status}
									isError={item.isError}
									errorMessage={item.errorMessage}
									expanded={expandedFileIndexes.has(index)}
									onExpandedChange={(nextExpanded) => {
										setExpandedFileIndexes((previous) => {
											const next = new Set(previous);
											if (nextExpanded) {
												next.add(index);
											} else {
												next.delete(index);
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
