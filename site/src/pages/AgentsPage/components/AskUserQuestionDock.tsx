import { cn } from "cn";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { MergedTool } from "./ChatConversation/types";
import { AskUserQuestionRenderer } from "./ChatElements/tools/Tool";

type AskUserQuestionDockProps = {
	question: MergedTool;
	onSubmitAnswer?: (message: string) => Promise<void> | void;
};

/**
 * Pins the open async question's answer form above the chat input. The form
 * always acts as the latest question, so pass only the question that
 * deriveAskUserQuestionState reports as open.
 */
export const AskUserQuestionDock: React.FC<AskUserQuestionDockProps> = ({
	question,
	onSubmitAnswer,
}) => {
	const [chatFullWidth] = useChatFullWidth();

	return (
		<div className="shrink-0 px-4">
			{/* Bounded so a long question set never pushes the input off a short screen. */}
			<section
				aria-label="Open question"
				className={cn(
					"mx-auto mb-2 max-h-[50vh] w-full overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary p-3 shadow-sm scrollbar-thin",
					chatWidthClass(chatFullWidth),
				)}
			>
				<AskUserQuestionRenderer
					toolCallId={question.id}
					args={question.args}
					result={question.result}
					status={question.status}
					isError={question.isError}
					isLatestAskUserQuestion
					onSendAskUserQuestionResponse={onSubmitAnswer}
				/>
			</section>
		</div>
	);
};
