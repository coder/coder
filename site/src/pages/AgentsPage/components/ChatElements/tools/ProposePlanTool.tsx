import { LoaderIcon, PlayIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { useMutation, useQuery } from "react-query";
import { API } from "#/api/api";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Response } from "../Response";
import type { ToolStatus } from "./utils";

export const ProposePlanTool: React.FC<{
	content?: string;
	fileID?: string;
	path: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	onImplementPlan?: () => Promise<void> | void;
}> = ({
	content: inlineContent,
	fileID,
	path,
	status,
	isError,
	errorMessage,
	onImplementPlan,
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
	const hasDisplayContent = displayContent.trim().length > 0;
	const implementPlanMutation = useMutation({
		mutationFn: async () => {
			if (!onImplementPlan) return;
			await onImplementPlan();
		},
	});
	const canImplementPlan =
		status === "completed" &&
		!effectiveError &&
		!fetchLoading &&
		hasDisplayContent &&
		Boolean(onImplementPlan);

	return (
		<div className="w-full">
			<div className="flex items-center gap-1.5 py-0.5 text-content-secondary">
				<span className="text-[13px]">
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
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-current" />
				)}
			</div>
			{hasDisplayContent ? (
				<>
					<Response>{displayContent}</Response>
					<div className="flex items-center gap-2">
						<CopyButton text={displayContent} label="Copy plan" />
						{canImplementPlan && (
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										type="button"
										variant="subtle"
										size="sm"
										onClick={() => {
											implementPlanMutation.mutate();
										}}
										disabled={
											!canImplementPlan || implementPlanMutation.isPending
										}
										aria-label="Implement plan"
									>
										{implementPlanMutation.isPending ? (
											<LoaderIcon className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />
										) : (
											<PlayIcon />
										)}
										{implementPlanMutation.isPending
											? "Implementing..."
											: "Implement"}
									</Button>
								</TooltipTrigger>
								<TooltipContent>Implement plan</TooltipContent>
							</Tooltip>
						)}
					</div>
				</>
			) : (
				!fetchLoading &&
				!effectiveError && (
					<p className="text-[13px] text-content-secondary italic">
						No plan content.
					</p>
				)
			)}
			{fetchLoading && (
				<div className="flex items-center gap-1.5 py-2 text-[13px] text-content-secondary">
					<LoaderIcon className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />
					Loading plan…
				</div>
			)}
		</div>
	);
};
