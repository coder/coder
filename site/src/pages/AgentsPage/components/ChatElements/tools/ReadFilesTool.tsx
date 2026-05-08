import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { MergedTool } from "../../ChatConversation/types";
import { getReadFileToolData, ReadFileContent } from "./ReadFileTool";
import { ToolCollapsible } from "./ToolCollapsible";

type ReadFileItem = {
	id: string;
	path: string;
	content: string;
	isError: boolean;
	errorMessage?: string;
};

const getReadFileItem = (tool: MergedTool): ReadFileItem => ({
	id: tool.id,
	...getReadFileToolData(tool),
});

export const ReadFilesTool: React.FC<{
	tools: readonly MergedTool[];
	expanded?: boolean;
	onExpandedChange?: (expanded: boolean) => void;
}> = ({ tools, expanded, onExpandedChange }) => {
	const items = tools.map(getReadFileItem);
	const isRunning = tools.some((tool) => tool.status === "running");
	const isError = tools.some((tool) => tool.isError);
	const hasContent = items.length > 0;
	const label = isRunning
		? `Reading ${tools.length} files…`
		: `Read ${tools.length} files`;
	const errorMessage = items.find((item) => item.errorMessage)?.errorMessage;

	return (
		<div
			data-tool-call=""
			className="py-0.5 [&:has(+[data-tool-call])]:pb-0 [[data-tool-call]+&]:pt-0"
		>
			<ToolCollapsible
				className="w-full"
				hasContent={hasContent}
				expanded={expanded}
				onExpandedChange={onExpandedChange}
				header={
					<>
						<span className="text-[13px]">{label}</span>
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
				<div className="space-y-3">
					{items.map((item) => (
						<section key={item.id} className="min-w-0">
							<div className="mt-2 truncate text-xs text-content-secondary">
								{item.path}
							</div>
							{item.isError && (
								<div className="mt-1 text-xs text-content-destructive">
									{item.errorMessage || "Failed to read file"}
								</div>
							)}
							{item.content.length > 0 && (
								<ReadFileContent path={item.path} content={item.content} />
							)}
						</section>
					))}
				</div>
			</ToolCollapsible>
		</div>
	);
};
