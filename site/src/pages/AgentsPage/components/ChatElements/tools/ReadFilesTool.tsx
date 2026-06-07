import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, useState } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { MergedTool } from "../../ChatConversation/types";
import { getReadFileToolData, ReadFileTool } from "./ReadFileTool";
import { ToolCollapsible } from "./ToolCollapsible";
import { ToolIcon } from "./ToolIcon";

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
			<ToolCollapsible
				className="w-full"
				hasContent={hasContent}
				expanded={expanded}
				onExpandedChange={onExpandedChange}
				header={
					<>
						<ToolIcon
							name="read_file"
							isError={isError}
							isRunning={isRunning}
						/>
						<span className="text-[13px] leading-6">{label}</span>
						{isError && (
							<Tooltip>
								<TooltipTrigger asChild>
									<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-current" />
								</TooltipTrigger>
								<TooltipContent>
									{errorMessage || "Failed to read one or more files"}
								</TooltipContent>
							</Tooltip>
						)}
						{isRunning && (
							<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
						)}
					</>
				}
			>
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
			</ToolCollapsible>
		</div>
	);
};
