import { cn } from "cn";
import { useId, useRef } from "react";
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
	const labelId = useId();
	const sectionRef = useRef<HTMLElement>(null);

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
				ref={sectionRef}
				aria-labelledby={labelId}
				className={cn(
					"mx-auto mb-2 max-h-[50vh] w-full overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary p-3 shadow-sm scrollbar-thin",
					chatWidthClass(chatFullWidth),
				)}
			>
				<p
					id={labelId}
					className="mb-2 text-xs font-medium text-content-secondary"
				>
					Open question
				</p>
				<AskUserQuestionForm
					questions={getAskUserQuestions(question.args, question.result)}
					previousResponseText={getAsyncAnswerBody(
						queuedAnswerText,
						question.id,
					)}
					showsSubmittedAnswer
					onSubmitAnswer={onSubmitAnswer ? submitAnswer : undefined}
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
