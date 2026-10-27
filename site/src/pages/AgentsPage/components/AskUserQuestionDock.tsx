import { cn } from "cn";
import { useId, useRef } from "react";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { MergedTool } from "./ChatConversation/types";
import { AskUserQuestionRenderer } from "./ChatElements/tools/Tool";

type AskUserQuestionDockProps = {
	question: MergedTool;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

/** Callers must pass the question that deriveAskUserQuestionState reports as open. */
export const AskUserQuestionDock: React.FC<AskUserQuestionDockProps> = ({
	question,
	onSubmitAnswer,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const labelId = useId();
	const sectionRef = useRef<HTMLElement>(null);

	return (
		// The stable scrollbar gutter matches the chat input's wrapper, so the
		// panel lines up with the input where scrollbars take up space.
		<div className="flex min-h-0 flex-col overflow-y-auto px-4 scrollbar-gutter-stable scrollbar-thin">
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
				<AskUserQuestionRenderer
					toolCallId={question.id}
					args={question.args}
					result={question.result}
					status={question.status}
					isError={question.isError}
					isLatestAskUserQuestion
					onSendAskUserQuestionResponse={onSubmitAnswer}
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
