/** One question's unsent answer. */
export type QuestionDraft = {
	/** An option's index, the "Other" field, or a skipped question. */
	selection: number | "other" | "skipped" | undefined;
	/** Kept while another option is selected, so switching back restores it. */
	otherText: string;
};

/** The unsent answers to a question set and the question on screen. */
export type FormDraft = {
	drafts: QuestionDraft[];
	questionIndex: number;
	answeredQuestionCount: number;
};

type DraftQuestion = { readonly options: readonly unknown[] };

const storageKeyPrefix = "agents.ask-user-question-draft.";

const isQuestionDraft = (
	value: unknown,
	optionCount: number,
): value is QuestionDraft => {
	if (typeof value !== "object" || value === null) {
		return false;
	}
	const { selection, otherText } = value as Record<string, unknown>;
	if (typeof otherText !== "string") {
		return false;
	}
	return (
		selection === undefined ||
		selection === "other" ||
		selection === "skipped" ||
		(Number.isInteger(selection) &&
			(selection as number) >= 0 &&
			(selection as number) < optionCount)
	);
};

const isCount = (value: unknown, max: number): value is number =>
	Number.isInteger(value) && (value as number) >= 0 && (value as number) <= max;

/**
 * Returns the draft saved for these questions. A draft saved for other
 * questions, for example under a call ID that a provider reused, is ignored.
 */
export const readFormDraft = (
	draftKey: string,
	questions: readonly DraftQuestion[],
): FormDraft | undefined => {
	const raw = localStorage.getItem(storageKeyPrefix + draftKey);
	if (raw === null) {
		return undefined;
	}
	let stored: unknown;
	try {
		stored = JSON.parse(raw);
	} catch {
		// A draft that does not parse starts the form over.
		return undefined;
	}
	if (typeof stored !== "object" || stored === null) {
		return undefined;
	}
	const { questionsJSON, drafts, questionIndex, answeredQuestionCount } =
		stored as Record<string, unknown>;
	if (
		questionsJSON !== JSON.stringify(questions) ||
		!Array.isArray(drafts) ||
		drafts.length !== questions.length ||
		!drafts.every((draft, index) =>
			isQuestionDraft(draft, questions[index].options.length),
		) ||
		!isCount(questionIndex, questions.length - 1) ||
		!isCount(answeredQuestionCount, questions.length)
	) {
		return undefined;
	}
	return { drafts, questionIndex, answeredQuestionCount };
};

export const saveFormDraft = (
	draftKey: string,
	questions: readonly DraftQuestion[],
	formDraft: FormDraft,
): void => {
	localStorage.setItem(
		storageKeyPrefix + draftKey,
		JSON.stringify({ questionsJSON: JSON.stringify(questions), ...formDraft }),
	);
};

export const removeFormDraft = (draftKey: string): void => {
	localStorage.removeItem(storageKeyPrefix + draftKey);
};
