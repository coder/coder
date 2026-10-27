import { cn } from "cn";
import { useEffect, useId, useRef } from "react";
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
	const labelId = useId();
	const sectionRef = useRef<HTMLElement>(null);
	// Providers can reuse a call ID, so the start time tells the calls apart
	// and a reused ID still gets a fresh form.
	const questionKey = `${question.id}:${question.startedAt ?? ""}`;
	const shownQuestionKeyRef = useRef(questionKey);
	useEffect(() => {
		if (shownQuestionKeyRef.current === questionKey) {
			return;
		}
		shownQuestionKeyRef.current = questionKey;
		const section = sectionRef.current;
		if (!section) {
			return;
		}
		section.scrollTop = 0;
		// A newer question replaced the form, and focus inside it fell to the
		// page body. Focus elsewhere, such as in the chat input, stays put. The
		// region takes focus, so no radio or text input opens the keyboard.
		if (document.activeElement === document.body) {
			section.focus({ preventScroll: true });
		}
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
			    pushes the input off a short screen. */}
			<section
				ref={sectionRef}
				aria-labelledby={labelId}
				tabIndex={-1}
				className={cn(
					"mx-auto mb-2 max-h-[50vh] w-full overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary p-3 shadow-sm scrollbar-thin focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
					chatWidthClass(chatFullWidth),
				)}
			>
				<p
					id={labelId}
					className="mb-2 text-xs font-medium text-content-secondary"
				>
					Open question
				</p>
				<AskUserQuestionRenderer
					key={questionKey}
					toolCallId={question.id}
					args={question.args}
					result={question.result}
					status={question.status}
					isError={question.isError}
					isLatestAskUserQuestion
					previousResponseText={queuedAnswerText}
					onSendAskUserQuestionResponse={onSubmitAnswer}
					announcesQuestion={false}
					onStepChange={() => {
						if (sectionRef.current) {
							sectionRef.current.scrollTop = 0;
						}
					}}
				/>
			</section>
		</div>
	);
};
