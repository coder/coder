import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";
import { Response } from "../response";
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";

export const ProposePlanTool: React.FC<{
	content: string;
	path: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ content, path, status, isError, errorMessage }) => {
	const hasContent = (content ?? "").trim().length > 0;
	const isRunning = status === "running";
	const filename = (path || "PLAN.md").split("/").pop() || "PLAN.md";

	return (
		<ToolCollapsible
			className="w-full"
			hasContent={hasContent}
			defaultExpanded
			header={
				<>
					<span
						className={cn(
							"text-sm",
							isError ? "text-content-destructive" : "text-content-secondary",
						)}
					>
						{isRunning ? `Proposing ${filename}…` : `Proposed ${filename}`}
					</span>
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon
									aria-label="Error"
									className="h-3.5 w-3.5 shrink-0 text-content-destructive"
								/>
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Failed to propose plan"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<div className="mt-1.5 rounded-md border border-solid border-border-default px-3 py-2">
				<Response>{content}</Response>
			</div>
		</ToolCollapsible>
	);
};
