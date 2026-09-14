import { TriangleAlertIcon } from "lucide-react";
import type React from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Response } from "../Response";
import { ToolCall } from "./ToolCall";
import { formatModelIntentLabel, type ToolStatus } from "./utils";

export type AdvisorToolResultType = "advice" | "limit_reached" | "error";

type AdvisorToolProps = {
	question: string;
	status: ToolStatus;
	isError: boolean;
	resultType?: AdvisorToolResultType;
	advice?: string;
	errorMessage?: string;
	modelIntent?: string;
};

export const AdvisorTool: React.FC<AdvisorToolProps> = ({
	question,
	status,
	isError,
	resultType,
	advice,
	errorMessage,
	modelIntent,
}) => {
	const questionText = question.trim() || "No question provided.";
	const adviceText = advice?.trim() ?? "";
	const effectiveErrorMessage =
		errorMessage?.trim() || "Advisor could not return guidance.";
	const isRunning = status === "running";
	const showLimitReached = resultType === "limit_reached";
	const showError = isError || resultType === "error";

	const intent = formatModelIntentLabel(modelIntent);
	const label = showLimitReached
		? "Advisor limit reached"
		: intent && !showError
			? intent
			: isRunning
				? "Consulting the advisor"
				: showError
					? "Failed to consult the advisor"
					: "Consulted the advisor";

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={showError}
			errorMessage={effectiveErrorMessage}
			hasContent
			defaultExpanded={isRunning}
		>
			<ToolCall.Header
				iconName="advisor"
				label={label}
				secondaryLabel={
					showLimitReached ? (
						<TriangleAlertIcon className="size-3.5 shrink-0 text-content-warning" />
					) : null
				}
			/>
			<ToolCall.Content>
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					viewportTabIndex={0}
					viewportAriaLabel="Advisor response"
					scrollBarClassName="w-1.5"
				>
					<div className="space-y-2 px-3 py-2">
						<p className="m-0 whitespace-pre-wrap wrap-break-word text-[13px] italic leading-5 text-content-secondary wrap-anywhere">
							{questionText}
						</p>
						<div className="border-0 border-t border-solid border-border-default pt-2">
							{showError ? (
								<div role="alert" className="text-sm">
									<p className="m-0 font-medium text-content-primary">
										Advisor request failed.
									</p>
									<p className="m-0 text-content-secondary wrap-anywhere">
										{effectiveErrorMessage}
									</p>
								</div>
							) : showLimitReached ? (
								<div role="status" className="text-sm">
									<p className="m-0 font-medium text-content-primary">
										Advisor limit reached.
									</p>
									<p className="m-0 text-content-secondary">
										You have reached the advisor limit for this conversation.
									</p>
								</div>
							) : isRunning && adviceText.length === 0 ? (
								<div
									role="status"
									className="text-[13px] text-content-secondary"
								>
									Reviewing context and preparing guidance.
								</div>
							) : (
								<Response
									streaming={isRunning}
									className="text-[13px] leading-5"
								>
									{adviceText || "Advisor returned no guidance."}
								</Response>
							)}
						</div>
					</div>
				</ScrollArea>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
