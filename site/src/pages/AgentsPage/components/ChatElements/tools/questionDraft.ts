import { asRecord } from "../runtimeTypeUtils";
import { parseJSON } from "./utils";

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

/**
 * Names one ask_user_question call. Providers can reuse a call ID, so the
 * start time tells the calls apart.
 */
export const getDraftKey = (toolCallId: string, startedAt?: string): string =>
	`${toolCallId}:${startedAt ?? ""}`;

const isIntegerInRange = (value: unknown, max: number): value is number =>
	typeof value === "number" &&
	Number.isInteger(value) &&
	value >= 0 &&
	value <= max;

const isQuestionDraft = (
	value: unknown,
	optionCount: number,
): value is QuestionDraft => {
	const record = asRecord(value);
	if (!record || typeof record.otherText !== "string") {
		return false;
	}
	const { selection } = record;
	return (
		selection === undefined ||
		selection === "other" ||
		selection === "skipped" ||
		isIntegerInRange(selection, optionCount - 1)
	);
};

/** Returns undefined for a malformed draft or one saved for other questions. */
export const readFormDraft = (
	draftKey: string,
	questions: readonly DraftQuestion[],
): FormDraft | undefined => {
	const raw = localStorage.getItem(storageKeyPrefix + draftKey);
	const stored = raw === null ? null : asRecord(parseJSON(raw));
	if (!stored) {
		return undefined;
	}
	const { questionsJSON, drafts, questionIndex, answeredQuestionCount } =
		stored;
	if (
		questionsJSON !== JSON.stringify(questions) ||
		!Array.isArray(drafts) ||
		drafts.length !== questions.length ||
		!drafts.every((draft: unknown, index) =>
			isQuestionDraft(draft, questions[index].options.length),
		) ||
		!isIntegerInRange(questionIndex, questions.length - 1) ||
		!isIntegerInRange(answeredQuestionCount, questions.length)
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
