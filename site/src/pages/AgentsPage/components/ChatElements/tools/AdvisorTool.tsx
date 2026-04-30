import { CircleAlertIcon, LoaderIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../Response";
import { ToolCollapsible } from "./ToolCollapsible";
import { ToolIcon } from "./ToolIcon";
import { ToolLabel } from "./ToolLabel";
import type { ToolStatus } from "./utils";

export type AdvisorToolResultType = "advice" | "limit_reached" | "error";

type AdvisorToolProps = {
	question: string;
	status: ToolStatus;
	isError: boolean;
	resultType?: AdvisorToolResultType;
	advice?: string;
	errorMessage?: string;
	advisorModel?: string;
	remainingUses?: number;
};

const FALLBACK_QUESTION = "No question provided.";
const FALLBACK_ERROR = "Advisor could not return guidance.";
const LIMIT_REACHED_MESSAGE =
	"You have reached the advisor limit for this conversation.";
const RUNNING_MESSAGE = "Consulting advisor…";
const EMPTY_ADVICE_MESSAGE = "Advisor returned no guidance.";

export const AdvisorTool: React.FC<AdvisorToolProps> = ({
	question,
	status,
	isError,
	resultType,
	advice,
	errorMessage,
	advisorModel,
	remainingUses,
}) => {
	const questionText = question.trim() || FALLBACK_QUESTION;
	const adviceText = advice?.trim() ?? "";
	const advisorModelText = advisorModel?.trim() ?? "";
	const effectiveErrorMessage = errorMessage?.trim() || FALLBACK_ERROR;
	const isRunning = status === "running";
	const showLimitReached = resultType === "limit_reached";
	const showError = isError || resultType === "error";
	const hasAdvice = resultType === "advice" && adviceText.length > 0;
	const hasMetadata =
		advisorModelText.length > 0 || remainingUses !== undefined;

	const headerStatus = isRunning
		? RUNNING_MESSAGE
		: showLimitReached
			? "Limit reached"
			: showError
				? "Request failed"
				: hasAdvice
					? "Guidance ready"
					: "No guidance";

	return (
		<ToolCollapsible
			className="w-full"
			hasContent
			defaultExpanded
			headerClassName="items-start"
			header={
				<>
					<ToolIcon name="advisor" isError={showError} isRunning={isRunning} />
					<div className="flex min-w-0 flex-1 flex-col gap-0.5">
						<div className="flex min-w-0 items-center gap-2">
							<ToolLabel
								name="advisor"
								args={{ question: questionText }}
								result={resultType ? { type: resultType } : undefined}
							/>
							<span className="text-2xs text-content-secondary">
								{headerStatus}
							</span>
						</div>
						<span className="block truncate text-sm text-content-primary">
							{questionText}
						</span>
					</div>
					{showLimitReached ? (
						<TriangleAlertIcon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-content-warning" />
					) : showError ? (
						<CircleAlertIcon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-content-destructive" />
					) : isRunning ? (
						<LoaderIcon className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					) : null}
				</>
			}
		>
			<ScrollArea
				className="mt-1.5 rounded-md border border-solid border-border-default bg-surface-primary"
				viewportClassName="max-h-64"
				scrollBarClassName="w-1.5"
				data-testid="advisor-tool-scroll-area"
			>
				<div className="space-y-3 px-3 py-2">
					{isRunning ? (
						<div className="flex items-center gap-2 text-sm text-content-secondary">
							<LoaderIcon className="h-4 w-4 shrink-0 animate-spin motion-reduce:animate-none" />
							<span>{RUNNING_MESSAGE}</span>
						</div>
					) : showLimitReached ? (
						<div
							role="status"
							className="flex items-start gap-3 rounded-md border border-solid border-border-warning bg-surface-orange p-3 text-sm text-content-primary"
						>
							<TriangleAlertIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-warning" />
							<div className="space-y-1">
								<p className="m-0 font-medium">Advisor limit reached.</p>
								<p className="m-0 text-content-primary">
									{LIMIT_REACHED_MESSAGE}
								</p>
							</div>
						</div>
					) : showError ? (
						<div
							role="alert"
							className="flex items-start gap-3 rounded-md border border-solid border-border-destructive bg-surface-red p-3 text-sm text-content-primary"
						>
							<CircleAlertIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-destructive" />
							<div className="space-y-1">
								<p className="m-0 font-medium">Advisor request failed.</p>
								<p className="m-0 text-content-primary [overflow-wrap:anywhere]">
									{effectiveErrorMessage}
								</p>
							</div>
						</div>
					) : (
						<div className="space-y-3">
							<Response>{adviceText || EMPTY_ADVICE_MESSAGE}</Response>
							{hasMetadata && (
								<div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-solid border-border-default pt-2 text-2xs text-content-secondary">
									{advisorModelText && (
										<span>
											Advisor model:{" "}
											<span className="font-medium text-content-primary">
												{advisorModelText}
											</span>
										</span>
									)}
									{remainingUses !== undefined && (
										<span>
											Remaining uses:{" "}
											<span className="font-medium text-content-primary">
												{remainingUses.toLocaleString("en-US")}
											</span>
										</span>
									)}
								</div>
							)}
						</div>
					)}
				</div>
			</ScrollArea>
		</ToolCollapsible>
	);
};
