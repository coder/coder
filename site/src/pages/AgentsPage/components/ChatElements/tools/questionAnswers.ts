import type { AskUserQuestion } from "./AskUserQuestionTool";
import type { QuestionDraft } from "./questionDraft";

export const OTHER_OPTION_VALUE = "other";

export const getQuestionHeader = (
	question: AskUserQuestion,
	questionIndex: number,
): string => question.header || `Question ${questionIndex + 1}`;

export const getQuestionLabel = (
	question: AskUserQuestion,
	questionIndex: number,
	questionCount: number,
): string => {
	const progress =
		questionCount > 1
			? `Question ${questionIndex + 1} of ${questionCount}`
			: "";
	return [progress, question.header].filter(Boolean).join(" · ") || "Question";
};

export const getQuestionText = (question: AskUserQuestion): string =>
	question.question || "No question provided.";

export const getQuestionKey = (
	question: AskUserQuestion,
	questionIndex: number,
): string => `${question.header}-${question.question}-${questionIndex}`;

export const filterQuestionOptions = (
	question: AskUserQuestion,
): AskUserQuestion => ({
	...question,
	options: question.options.filter(
		(option) => option.label.trim().toLowerCase() !== "other",
	),
});

export const getOptionLabel = (
	question: AskUserQuestion,
	optionIndex: number,
) => question.options[optionIndex].label || `Option ${optionIndex + 1}`;

export const getDefaultDraft = (question: AskUserQuestion): QuestionDraft => ({
	selection: question.options.length > 0 ? 0 : undefined,
	otherText: "",
});

type AnsweredDraft = QuestionDraft & {
	selection: Exclude<QuestionDraft["selection"], undefined>;
};

export const isDraftAnswered = (
	draft: QuestionDraft,
): draft is AnsweredDraft => {
	if (draft.selection === undefined) {
		return false;
	}
	if (draft.selection === "other") {
		return draft.otherText.trim().length > 0;
	}
	return true;
};

const formatAnswer = (
	question: AskUserQuestion,
	draft: AnsweredDraft,
): string => {
	if (draft.selection === "other") {
		return `Other: ${draft.otherText.trim()}`;
	}
	if (draft.selection === "skipped") {
		return "Skipped";
	}
	return getOptionLabel(question, draft.selection);
};

export const formatOutgoingMessage = (
	questions: AskUserQuestion[],
	drafts: readonly AnsweredDraft[],
): string => {
	if (questions.length === 1) {
		return formatAnswer(questions[0], drafts[0]);
	}

	return questions
		.map((question, questionIndex) => {
			return `${questionIndex + 1}. ${getQuestionHeader(question, questionIndex)}: ${formatAnswer(question, drafts[questionIndex])}`;
		})
		.join("\n");
};
