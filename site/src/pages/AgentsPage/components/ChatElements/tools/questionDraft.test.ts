import { afterEach, describe, expect, it } from "vitest";
import { type FormDraft, readFormDraft, saveFormDraft } from "./questionDraft";

const mockQuestions = [
	{
		header: "Approach",
		question: "Which one?",
		options: [{ label: "A", description: "The first approach." }],
	},
	{ header: "Rollout", question: "How?", options: [] },
];

const mockFormDraft: FormDraft = {
	drafts: [
		{ selection: 0, otherText: "" },
		{ selection: "skipped", otherText: "later" },
	],
	questionIndex: 1,
};

afterEach(() => {
	localStorage.clear();
});

describe("readFormDraft", () => {
	it("returns the draft saved for the same questions", () => {
		saveFormDraft("call-1", mockQuestions, mockFormDraft);

		expect(readFormDraft("call-1", mockQuestions)).toEqual(mockFormDraft);
	});

	it.each([
		{ name: "different questions", raw: undefined, read: [mockQuestions[0]] },
		{ name: "malformed JSON", raw: "{", read: mockQuestions },
		{
			name: "an option index out of range",
			raw: JSON.stringify({
				...mockFormDraft,
				questions: mockQuestions,
				drafts: [mockFormDraft.drafts[0], { selection: 0, otherText: "" }],
			}),
			read: mockQuestions,
		},
		{
			name: "a question index out of range",
			raw: JSON.stringify({
				...mockFormDraft,
				questions: mockQuestions,
				questionIndex: 2,
			}),
			read: mockQuestions,
		},
	])("ignores a draft with $name", ({ raw, read }) => {
		saveFormDraft("call-1", mockQuestions, mockFormDraft);
		if (raw !== undefined) {
			localStorage.setItem("agents.ask-user-question-draft.call-1", raw);
		}

		expect(readFormDraft("call-1", read)).toBeUndefined();
	});
});
