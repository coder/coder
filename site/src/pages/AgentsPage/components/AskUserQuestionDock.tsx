import { cn } from "cn";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { MergedTool } from "./ChatConversation/types";
import { AskUserQuestionRenderer } from "./ChatElements/tools/Tool";

type AskUserQuestionDockProps = {
	question: MergedTool;
	/**
	 * Hides the dock while the answer waits in the queue. The dock stays
	 * mounted so its draft is still there if the queue drops the answer.
	 */
	queuedAnswerText: string | undefined;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

/** Callers must pass the question that deriveAskUserQuestionState reports as open. */
export const AskUserQuestionDock: React.FC<AskUserQuestionDockProps> = ({
	question,
	queuedAnswerText,
	onSubmitAnswer,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const sectionRef = useRef<HTMLElement | null>(null);
	const scrollerRef = useRef<HTMLDivElement>(null);
	const replacedFocusedSectionRef = useRef(false);
	// Providers can reuse a call ID, so the start time tells the calls apart.
	// Keying the section by it gives a newer question a fresh form that starts
	// scrolled to the top.
	const questionKey = `${question.id}:${question.startedAt ?? ""}`;
	// Keyed by question, so a newer question always opens expanded.
	const [collapsedQuestionKey, setCollapsedQuestionKey] = useState<
		string | undefined
	>();
	const isCollapsed = collapsedQuestionKey === questionKey;
	useEffect(() => {
		if (!replacedFocusedSectionRef.current) {
			return;
		}
		replacedFocusedSectionRef.current = false;
		// The focus fell to the page body with the replaced section. The region
		// takes it, so screen readers announce "Open question" and Tab reaches
		// the form.
		sectionRef.current?.focus({ preventScroll: true });
	}, [questionKey]);

	return (
		// The stable scrollbar gutter matches the chat input's wrapper, so the
		// panel lines up with the input where scrollbars take up space.
		// `hidden` wins over `flex` only through Tailwind preflight's
		// `[hidden]` rule.
		<div
			hidden={queuedAnswerText !== undefined}
			className="flex min-h-0 flex-col overflow-y-auto px-4 scrollbar-gutter-stable scrollbar-thin"
		>
			{/* Shrinks before the chat input does, so a long question set never
			    pushes the input off a short screen. The chevron sits outside the
			    scroller, so it stays in place while the form scrolls. */}
			<section
				key={questionKey}
				ref={(section) => {
					sectionRef.current = section;
					return () => {
						// React detaches a ref before it removes the node, so a
						// section that a newer question replaces still holds focus.
						replacedFocusedSectionRef.current =
							section?.contains(document.activeElement) ?? false;
						sectionRef.current = null;
					};
				}}
				aria-label="Open question"
				tabIndex={-1}
				className={cn(
					"mx-auto mb-2 flex max-h-[50vh] w-full overflow-hidden rounded-lg border border-solid border-border bg-surface-primary shadow-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
					chatWidthClass(chatFullWidth),
				)}
			>
				<div
					ref={scrollerRef}
					className="min-w-0 flex-1 overflow-y-auto py-3 pr-1 pl-3 scrollbar-thin"
				>
					<AskUserQuestionRenderer
						toolCallId={question.id}
						args={question.args}
						result={question.result}
						status={question.status}
						isError={question.isError}
						isLatestAskUserQuestion
						previousResponseText={queuedAnswerText}
						onSendAskUserQuestionResponse={onSubmitAnswer}
						announcesQuestion={false}
						isCollapsed={isCollapsed}
						hasStickyActions
						onStepChange={() => {
							if (scrollerRef.current) {
								scrollerRef.current.scrollTop = 0;
							}
						}}
					/>
				</div>
				<Button
					type="button"
					size="icon"
					variant="subtle"
					className="m-1.5 shrink-0"
					aria-expanded={!isCollapsed}
					aria-label={isCollapsed ? "Expand question" : "Collapse question"}
					onClick={() => {
						setCollapsedQuestionKey(isCollapsed ? undefined : questionKey);
					}}
				>
					{isCollapsed ? <ChevronUpIcon /> : <ChevronDownIcon />}
				</Button>
			</section>
		</div>
	);
};
