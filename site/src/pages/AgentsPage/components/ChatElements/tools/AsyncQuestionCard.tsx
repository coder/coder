import { useId } from "react";
import type { AsyncQuestionStatus } from "../../ChatConversation/asyncQuestions";
import {
	AnsweredQuestionText,
	type AskUserQuestion,
	NoQuestionsAvailable,
	QuestionErrorView,
	QuestionRunningHeader,
	SubmittedAnswer,
} from "./AskUserQuestionTool";
import type { ToolStatus } from "./utils";

type AsyncQuestionCardProps = {
	questions: AskUserQuestion[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
	/** Undefined while the call has no question_id or its state is not known. */
	questionStatus: AsyncQuestionStatus | undefined;
	answerText: string | undefined;
	/** The question panel shows this question. */
	isInQuestionPanel: boolean;
};

export const AsyncQuestionCard: React.FC<AsyncQuestionCardProps> = ({
	questions,
	status,
	isError,
	errorMessage,
	questionStatus,
	answerText,
	isInQuestionPanel,
}) => {
	const idPrefix = useId();
	const isRunning = status === "running";

	if (isError) {
		return <QuestionErrorView status={status} errorMessage={errorMessage} />;
	}

	if (questions.length === 0) {
		return (
			<div className="w-full">
				{isRunning ? (
					<QuestionRunningHeader status={status} />
				) : (
					<NoQuestionsAvailable />
				)}
			</div>
		);
	}

	let statusText: string | undefined;
	if (questionStatus === "closed") {
		statusText = "This question is closed.";
	} else if (questionStatus === "open" && isInQuestionPanel) {
		statusText = "Answer in the question panel above the chat input.";
	}

	let footer: React.ReactNode = null;
	if (answerText !== undefined) {
		footer = <SubmittedAnswer text={answerText} />;
	} else if (statusText) {
		footer = (
			<p className="mt-3 text-[13px] text-content-secondary">{statusText}</p>
		);
	}

	return (
		<div className="w-full">
			{isRunning && <QuestionRunningHeader status={status} />}
			<div className="space-y-5">
				{questions.map((question, questionIndex) => (
					<AnsweredQuestionText
						key={`${question.header}-${question.question}-${questionIndex}`}
						question={question}
						questionIndex={questionIndex}
						idPrefix={idPrefix}
					/>
				))}
			</div>
			{footer}
		</div>
	);
};
