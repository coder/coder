import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Response } from "../Response";
import { ToolCollapsible } from "./ToolCollapsible";
import { ToolIcon } from "./ToolIcon";
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
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			header={
				<>
					<ToolIcon name="read_skill" isError={isError} isRunning={isRunning} />
					<span className="text-[13px] leading-6">
						{isRunning ? `Reading ${label}…` : `Read ${label}`}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="size-3.5 shrink-0 text-current" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to read skill"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
					)}
				</>
			}
		>
			{body && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{body}</Response>
					</div>
				</ScrollArea>
			)}
		</ToolCollapsible>
	);
};
