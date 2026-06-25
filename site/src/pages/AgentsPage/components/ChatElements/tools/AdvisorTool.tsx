import { CircleAlertIcon, TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { cn } from "#/utils/cn";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
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

	const headerStatus = showLimitReached ? (
		<TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0 text-content-warning" />
	) : showError ? (
		<CircleAlertIcon className="mt-0.5 size-3.5 shrink-0 text-content-destructive" />
	) : (
		<ToolCall.Status className="mt-0.5 text-content-secondary" />
	);

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={showError}
			errorMessage={effectiveErrorMessage}
			hasContent
			defaultExpanded
		>
			<ToolCall.HeaderButton className="items-start">
				<ToolCall.State>
					{({ expanded }) => (
						<>
							<div className="flex min-w-0 flex-1 flex-col gap-0.5">
								<div className="flex min-w-0 items-center gap-2 leading-4">
									<ToolCall.LeadingIcon name="advisor" />
									<ToolLabel
										name="advisor"
										args={{ question: questionText }}
										result={resultType ? { type: resultType } : undefined}
									/>
									{isRunning && (
										<span className="shrink-0 rounded-full border border-solid border-border-default px-2 text-[13px] leading-4 text-content-secondary">
											{RUNNING_MESSAGE}
										</span>
									)}
									{advisorModelText && (
										<span className="min-w-0 truncate rounded-full border border-solid border-border-default px-2 text-[13px] leading-4 text-content-secondary">
											{advisorModelText}
										</span>
									)}
									{remainingUses !== undefined && (
										<span className="shrink-0 rounded-full border border-solid border-border-default px-2 text-[13px] leading-4 text-content-secondary">
											{remainingUses.toLocaleString("en-US")} uses left
										</span>
									)}
								</div>
								<span
									className={cn(
										"ml-6 block whitespace-normal break-words text-[13px]",
										"font-normal leading-5 text-content-primary",
										"[overflow-wrap:anywhere]",
										!expanded && "line-clamp-2",
									)}
								>
									{questionText}
								</span>
							</div>
							{headerStatus}
							<ToolCall.Chevron className="mt-0.5" />
						</>
					)}
				</ToolCall.State>
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default bg-surface-primary"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
					data-testid="advisor-tool-scroll-area"
				>
					<div className="space-y-3 px-3 py-2">
						{isRunning && adviceText.length === 0 ? (
							<div role="status" className="text-sm text-content-secondary">
								Reviewing context and preparing guidance.
							</div>
						) : showLimitReached ? (
							<div
								role="status"
								className="flex items-start gap-3 rounded-md border border-solid border-border-warning bg-surface-orange p-3 text-sm text-content-primary"
							>
								<TriangleAlertIcon className="mt-0.5 size-4 shrink-0 text-content-warning" />
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
								<CircleAlertIcon className="mt-0.5 size-4 shrink-0 text-content-destructive" />
								<div className="space-y-1">
									<p className="m-0 font-medium">Advisor request failed.</p>
									<p className="m-0 text-content-primary [overflow-wrap:anywhere]">
										{effectiveErrorMessage}
									</p>
								</div>
							</div>
						) : (
							<section className="space-y-2" aria-label="Advisor advice">
								<div>
									<span className="inline-flex rounded-full border border-solid border-border-default px-2 text-[13px] leading-4 text-content-secondary">
										Advice
									</span>
								</div>
								<Response
									streaming={isRunning}
									className="[&_h1]:mb-2 [&_h1]:mt-3 [&_h1]:text-[15px] [&_h2]:mb-1.5 [&_h2]:mt-3 [&_h2]:text-sm [&_h3]:mb-1 [&_h3]:mt-2.5 [&_h3]:text-[13px] [&_h4]:mt-2 [&_h4]:text-[13px] [&_h5]:text-xs [&_h6]:text-xs"
								>
									{adviceText || EMPTY_ADVICE_MESSAGE}
								</Response>
							</section>
						)}
					</div>
				</ScrollArea>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
