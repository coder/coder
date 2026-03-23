import { API } from "api/api";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, LoaderIcon } from "lucide-react";
import type React from "react";
import { useEffect, useState } from "react";
import { cn } from "utils/cn";
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
	const [fetchedContent, setFetchedContent] = useState<string | undefined>();
	const [fetchError, setFetchError] = useState<string | undefined>();
	const [fetchLoading, setFetchLoading] = useState(false);

	const hasInlineContent = (inlineContent?.trim().length ?? 0) > 0;

	useEffect(() => {
		setFetchedContent(undefined);
		setFetchError(undefined);

		if (!fileID || hasInlineContent) {
			setFetchLoading(false);
			return;
		}

		let cancelled = false;
		setFetchLoading(true);

		API.experimental
			.getChatFileText(fileID)
			.then((text) => {
				if (!cancelled) {
					setFetchedContent(text);
					setFetchLoading(false);
				}
			})
			.catch((err) => {
				if (!cancelled) {
					setFetchError(
						err instanceof Error ? err.message : "Failed to load plan",
					);
					setFetchLoading(false);
				}
			});

		return () => {
			cancelled = true;
		};
	}, [fileID, hasInlineContent]);

	const displayContent = hasInlineContent
		? inlineContent || ""
		: fetchedContent || "";
	const isRunning = status === "running";
	const filename = (path || "PLAN.md").split("/").pop() || "PLAN.md";
	const effectiveError = isError || Boolean(fetchError);
	const effectiveErrorMessage = errorMessage || fetchError;

	return (
		<div className="w-full">
			{(isRunning || effectiveError) && (
				<div className="flex items-center gap-1.5 py-0.5">
					<span
						className={cn(
							"text-sm",
							effectiveError
								? "text-content-destructive"
								: "text-content-secondary",
						)}
					>
						{isRunning ? `Proposing ${filename}…` : `Proposed ${filename}`}
					</span>
					{effectiveError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<CircleAlertIcon
									aria-label="Error"
									className="h-3.5 w-3.5 shrink-0 text-content-destructive"
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
			)}
			{displayContent && <Response>{displayContent}</Response>}
			{fetchLoading && (
				<div className="flex items-center gap-1.5 py-2 text-sm text-content-secondary">
					<LoaderIcon className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />
					Loading plan…
				</div>
			)}
		</div>
	);
};
