import type { AskUserQuestion } from "./AskUserQuestionTool";
import type { QuestionDraft } from "./questionDraft";

/** The radio value of the free-text Other answer. */
export const OTHER_OPTION_VALUE = "other";

/** The question's header, or its position when it has none. */
export const getQuestionHeader = (
	question: AskUserQuestion,
	questionIndex: number,
): string => question.header || `Question ${questionIndex + 1}`;

/** The question's position in a set of several, then its header. */
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

/** The question text, or a placeholder when it is empty. */
export const getQuestionText = (question: AskUserQuestion): string =>
	question.question || "No question provided.";

/** A React key for a question within its set. */
export const getQuestionKey = (
	question: AskUserQuestion,
	questionIndex: number,
): string => `${question.header}-${question.question}-${questionIndex}`;

/** Drops options labeled Other, since the form adds its own Other field. */
export const filterQuestionOptions = (
	question: AskUserQuestion,
): AskUserQuestion => ({
	...question,
	options: question.options.filter(
		(option) => option.label.trim().toLowerCase() !== "other",
	),
});

/** The option's label, or its position when it has none. */
export const getOptionLabel = (
	question: AskUserQuestion,
	optionIndex: number,
) => question.options[optionIndex].label || `Option ${optionIndex + 1}`;

/** Selects the first option, if the question has any. */
export const getDefaultDraft = (question: AskUserQuestion): QuestionDraft => ({
	selection: question.options.length > 0 ? 0 : undefined,
	otherText: "",
});

type AnsweredDraft = QuestionDraft & {
	selection: Exclude<QuestionDraft["selection"], undefined>;
};

/** Reports whether the draft has a selection, with text when it is Other. */
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

/** Formats the answers as the message that the form sends. */
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
