import { afterEach, describe, expect, it } from "vitest";
import { type FormDraft, readFormDraft, saveFormDraft } from "./questionDraft";

const questions = [
	{ header: "Approach", question: "Which one?", options: [{ label: "A" }] },
	{ header: "Rollout", question: "How?", options: [] },
];

const formDraft: FormDraft = {
	drafts: [
		{ selection: 0, otherText: "" },
		{ selection: "skipped", otherText: "later" },
	],
	questionIndex: 1,
	answeredQuestionCount: 1,
};

afterEach(() => {
	localStorage.clear();
});

describe("readFormDraft", () => {
	it("returns the draft saved for the same questions", () => {
		saveFormDraft("call-1", questions, formDraft);

		expect(readFormDraft("call-1", questions)).toEqual(formDraft);
	});

	it.each([
		{ name: "different questions", raw: undefined, read: [questions[0]] },
		{ name: "malformed JSON", raw: "{", read: questions },
		{
			name: "an option index out of range",
			raw: JSON.stringify({
				...formDraft,
				questionsJSON: JSON.stringify(questions),
				drafts: [formDraft.drafts[0], { selection: 0, otherText: "" }],
			}),
			read: questions,
		},
		{
			name: "a question index out of range",
			raw: JSON.stringify({
				...formDraft,
				questionsJSON: JSON.stringify(questions),
				questionIndex: 2,
			}),
			read: questions,
		},
	])("ignores a draft with $name", ({ raw, read }) => {
		saveFormDraft("call-1", questions, formDraft);
		if (raw !== undefined) {
			localStorage.setItem("agents.ask-user-question-draft.call-1", raw);
		}

		expect(readFormDraft("call-1", read)).toBeUndefined();
	});
});
