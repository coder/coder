import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { API } from "#/api/api";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Response } from "../response";
import type { ToolStatus } from "./utils";

export const ProposePlanTool: React.FC<{
	content?: string;
	fileID?: string;
	path: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({
	content: inlineContent,
	fileID,
	path,
	status,
	isError,
	errorMessage,
}) => {
	const hasInlineContent = (inlineContent?.trim().length ?? 0) > 0;
	const fileQuery = useQuery({
		queryKey: ["chatFile", fileID],
		queryFn: async () => {
			if (!fileID) {
				throw new Error("Missing file ID");
			}

			return API.experimental.getChatFileText(fileID);
		},
		enabled: Boolean(fileID) && !hasInlineContent,
		staleTime: Number.POSITIVE_INFINITY,
	});

	const fetchError = fileQuery.isError
		? fileQuery.error instanceof Error
			? fileQuery.error.message
			: "Failed to load plan"
		: undefined;
	const fetchLoading = fileQuery.isLoading;
	const displayContent = hasInlineContent
		? (inlineContent ?? "")
		: (fileQuery.data ?? "");
	const isRunning = status === "running";
	const filename = (path || "PLAN.md").split("/").pop() || "PLAN.md";
	const effectiveError = isError || Boolean(fetchError);
	const effectiveErrorMessage = errorMessage || fetchError;

	return (
		<div className="w-full">
			<div className="flex items-center gap-1.5 py-0.5">
				<span className={cn("text-sm", "text-content-secondary")}>
					{isRunning ? `Proposing ${filename}…` : `Proposed ${filename}`}
				</span>
				{effectiveError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<TriangleAlertIcon
								aria-label="Error"
								className="h-3.5 w-3.5 shrink-0 text-content-secondary"
							/>
						</TooltipTrigger>
						<TooltipContent>
							{effectiveErrorMessage || "Failed to propose plan"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
				)}
			</div>
			{displayContent ? (
				<Response>{displayContent}</Response>
			) : (
				!fetchLoading &&
				!effectiveError && (
					<p className="text-sm text-content-secondary italic">
						No plan content.
					</p>
				)
			)}
			{fetchLoading && (
				<div className="flex items-center gap-1.5 py-2 text-sm text-content-secondary">
					<LoaderIcon className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />
					Loading plan…
				</div>
			)}
		</div>
	);
};
