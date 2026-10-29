import { cn } from "cn";
import { useRef, useState } from "react";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { MergedTool } from "./ChatConversation/types";
import { AskUserQuestionForm } from "./ChatElements/tools/AskUserQuestionTool";
import { getQuestionCallKey } from "./ChatElements/tools/questionDraft";
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
	// Keying the scroller by call gives a newer question a fresh form that
	// starts scrolled to the top.
	const questionKey = getQuestionCallKey(question.id, question.startedAt);
	// Keyed by question, so a newer question always opens expanded.
	const [collapsedQuestionKey, setCollapsedQuestionKey] = useState<
		string | undefined
	>();
	const isCollapsed = collapsedQuestionKey === questionKey;

	return (
		<div
			key={questionKey}
			// `hidden` wins over `flex` only through Tailwind preflight's `[hidden]` rule.
			hidden={queuedAnswerText !== undefined}
			ref={scrollerRef}
			// A stable scrollbar gutter, as on the chat input's wrapper, keeps the two aligned.
			className="flex max-h-[60vh] min-h-0 scroll-pb-14 flex-col overflow-y-auto px-4 scrollbar-gutter-stable scrollbar-thin"
		>
			{/* Shrinks before the chat input does, so a long question set never
			    pushes the input off a short screen. */}
			<section
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
					"mx-auto mb-2 w-full shrink-0 rounded-lg border border-solid border-border bg-surface-primary shadow-sm focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
					chatWidthClass(chatFullWidth),
				)}
			>
				<AskUserQuestionForm
					questions={getAskUserQuestions(question.args, question.result)}
					previousResponseText={queuedAnswerText}
					asyncToolCallId={question.id}
					draftKey={questionKey}
					showSubmittedAnswer
					onSubmitAnswer={onSubmitAnswer}
					isCollapsed={isCollapsed}
					onToggleCollapsed={() => {
						setCollapsedQuestionKey(isCollapsed ? undefined : questionKey);
					}}
					hasStickyActions
					onStepChange={() => {
						if (scrollerRef.current) {
							scrollerRef.current.scrollTop = 0;
						}
					}}
				/>
			</section>
		</div>
	);
};
