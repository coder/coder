import { cn } from "cn";
import {
	ChevronDownIcon,
	ChevronUpIcon,
	MessageCircleQuestionIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import type { AsyncQuestion } from "./ChatConversation/asyncQuestions";
import { AskUserQuestionForm } from "./ChatElements/tools/AskUserQuestionTool";
import { getAskUserQuestions } from "./ChatElements/tools/Tool";

type QuestionPanelProps = {
	/** The open or pending async question. */
	question: AsyncQuestion;
	/**
	 * Hides the panel without unmounting it, so the form keeps its
	 * selections.
	 */
	isHidden: boolean;
	/** Undefined while an answer cannot be sent. */
	onSubmitAnswer?: (
		message: string,
		questionId: string,
	) => Promise<void> | void;
};

export const QuestionPanel: React.FC<QuestionPanelProps> = ({
	question,
	isHidden,
	onSubmitAnswer,
}) => {
	const [chatFullWidth] = useChatFullWidth();
	const contentId = useId();
	const [isCollapsed, setIsCollapsed] = useState(false);
	const [isAwaitingDelivery, setIsAwaitingDelivery] = useState(false);

	// The queued messages now show the answer. From here the question's
	// status hides the panel, and brings it back if the user deletes the
	// queued answer.
	if (isAwaitingDelivery && question.status === "pending") {
		setIsAwaitingDelivery(false);
	}

	const hidden =
		isHidden || question.status === "pending" || isAwaitingDelivery;
	const questions = getAskUserQuestions(
		question.tool.args,
		question.tool.result,
	);

	return (
		<div
			// `hidden` wins over `flex` only through Tailwind preflight's `[hidden]` rule.
			hidden={hidden}
			className="flex max-h-[60vh] min-h-0 flex-col overflow-y-auto px-4 scrollbar-gutter-stable scrollbar-thin"
		>
			<section
				aria-label="Open question"
				className={cn(
					"mx-auto mb-2 w-full shrink-0 rounded-lg border border-solid border-border bg-surface-primary shadow-sm",
					chatWidthClass(chatFullWidth),
				)}
			>
				<button
					type="button"
					aria-expanded={!isCollapsed}
					aria-controls={contentId}
					className="flex w-full appearance-none items-center gap-1.5 px-3 py-2.5 text-left font-sans text-xs text-content-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link"
					onClick={() => {
						setIsCollapsed(!isCollapsed);
					}}
				>
					<MessageCircleQuestionIcon
						aria-hidden="true"
						className="size-3.5 shrink-0"
					/>
					<span className="min-w-0 flex-1 truncate">Open question</span>
					{isCollapsed ? (
						<ChevronUpIcon aria-hidden="true" className="size-4 shrink-0" />
					) : (
						<ChevronDownIcon aria-hidden="true" className="size-4 shrink-0" />
					)}
				</button>
				<div id={contentId} hidden={isCollapsed} className="px-3 pb-3">
					<AskUserQuestionForm
						questions={questions}
						onSubmitAnswer={
							onSubmitAnswer
								? (message) => onSubmitAnswer(message, question.questionId)
								: undefined
						}
						onSubmitted={() => {
							setIsAwaitingDelivery(true);
						}}
					/>
				</div>
			</section>
		</div>
	);
};
