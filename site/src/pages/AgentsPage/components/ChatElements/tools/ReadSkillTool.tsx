import { BookOpenIcon, LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { Response } from "../Response";
import { ToolCollapsible } from "./ToolCollapsible";
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
					<BookOpenIcon className="h-4 w-4 shrink-0 text-content-secondary" />
					<span className={cn("text-sm", "text-content-secondary")}>
						{isRunning ? `Reading ${label}…` : `Read ${label}`}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to read skill"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
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
