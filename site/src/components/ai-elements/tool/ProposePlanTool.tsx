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
import { ToolCollapsible } from "./ToolCollapsible";
import type { ToolStatus } from "./utils";

export const ProposePlanTool: React.FC<{
	content?: string;
	fileID?: string;
	path: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ content, fileID, path, status, isError, errorMessage }) => {
	const [fetchedContent, setFetchedContent] = useState<string | undefined>();
	const [fetchError, setFetchError] = useState<string | undefined>();
	const [loading, setLoading] = useState(false);

	const hasInlineContent = (content?.trim().length ?? 0) > 0;

	useEffect(() => {
		setFetchedContent(undefined);
		setFetchError(undefined);

		if (!fileID || hasInlineContent) {
			setLoading(false);
			return;
		}

		let cancelled = false;
		setLoading(true);

		API.experimental
			.getChatFileText(fileID)
			.then((text) => {
				if (!cancelled) {
					setFetchedContent(text);
					setLoading(false);
				}
			})
			.catch((err) => {
				if (!cancelled) {
					setFetchError(
						err instanceof Error ? err.message : "Failed to load plan",
					);
					setLoading(false);
				}
			});

		return () => {
			cancelled = true;
		};
	}, [fileID, hasInlineContent]);

	const displayContent = hasInlineContent
		? content || ""
		: fetchedContent || "";
	const hasContent = displayContent.trim().length > 0 || Boolean(fileID);
	const isRunning = status === "running";
	const filename = (path || "PLAN.md").split("/").pop() || "PLAN.md";
	const effectiveError = isError || Boolean(fetchError);
	const effectiveErrorMessage = errorMessage || fetchError;

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
					{(isRunning || loading) && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			<div className="mt-1.5 rounded-md border border-solid border-border-default px-3 py-2">
				{loading ? (
					<span className="text-sm text-content-secondary">Loading plan…</span>
				) : (
					<Response>{displayContent}</Response>
				)}
			</div>
		</ToolCollapsible>
	);
};
