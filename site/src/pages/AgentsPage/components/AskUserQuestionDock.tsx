import { cn } from "cn";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { MergedTool } from "./ChatConversation/types";
import { AskUserQuestionForm } from "./ChatElements/tools/AskUserQuestionTool";
import {
	getAsyncAnswerBody,
	getAsyncAnswerPrefix,
} from "./ChatElements/tools/asyncAnswer";
import { getAskUserQuestions } from "./ChatElements/tools/Tool";

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
	const scrollerRef = useRef<HTMLDivElement>(null);
	const hadFocusRef = useRef(false);
	// Providers can reuse a call ID, so the start time tells the calls apart.
	// Keying the section by it gives a newer question a fresh form that starts
	// scrolled to the top.
	const questionKey = `${question.id}:${question.startedAt ?? ""}`;
	// Keyed by question, so a newer question always opens expanded.
	const [collapsedQuestionKey, setCollapsedQuestionKey] = useState<
		string | undefined
	>();
	const isCollapsed = collapsedQuestionKey === questionKey;

	const submitAnswer = async (answerText: string) => {
		await onSubmitAnswer?.(getAsyncAnswerPrefix(question.id) + answerText);
	};

	return (
		<div
			// `hidden` wins over `flex` only through Tailwind preflight's `[hidden]` rule.
			hidden={queuedAnswerText !== undefined}
			// A stable scrollbar gutter, as on the chat input's wrapper, keeps the two aligned.
			className="flex min-h-0 flex-col overflow-y-auto px-4 scrollbar-gutter-stable scrollbar-thin"
		>
			{/* Shrinks before the chat input does, so a long question set never
			    pushes the input off a short screen. */}
			<section
				key={questionKey}
				// A section that replaces a focused one takes the focus, so screen
				// readers announce "Open question" and Tab reaches the new form.
				ref={(section: HTMLElement) => {
					if (hadFocusRef.current && document.activeElement === document.body) {
						section.focus({ preventScroll: true });
					}
					// React runs ref cleanups before it removes the node, so focus
					// inside it has not fallen to the page body yet.
					return () => {
						hadFocusRef.current = section.contains(document.activeElement);
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
					<AskUserQuestionForm
						questions={getAskUserQuestions(question.args, question.result)}
						previousResponseText={getAsyncAnswerBody(
							queuedAnswerText,
							question.id,
						)}
						showsSubmittedAnswer
						onSubmitAnswer={onSubmitAnswer ? submitAnswer : undefined}
						isCollapsed={isCollapsed}
						hasStickyActions
						onStepChange={() => {
							if (scrollerRef.current) {
								scrollerRef.current.scrollTop = 0;
							}
						}}
					/>
				</div>
				{/* Outside the scroller, so it stays in place while the form scrolls. */}
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
