import { describe, expect, it } from "vitest";
import {
	getAsyncAnswerPrefix,
	getAsyncAnswerText,
	parseAsyncAnswer,
} from "./asyncAnswer";

describe("parseAsyncAnswer", () => {
	// Providers choose call IDs, so they may contain colons or spaces.
	it.each(["toolu_123", "functions.ask_user_question:0", "call 123"])(
		"splits an answer to the call ID %j into the ID and its body",
		(toolCallId) => {
			expect(
				parseAsyncAnswer(
					`${getAsyncAnswerPrefix(toolCallId)}1. Scope: Backend`,
				),
			).toEqual({ toolCallId, body: "1. Scope: Backend" });
		},
	);

	it("reads a prefix line that is the whole text as an empty answer", () => {
		expect(
			parseAsyncAnswer("Answer to ask_user_question call toolu_123:"),
		).toEqual({ toolCallId: "toolu_123", body: "" });
	});

	it.each([
		["text without the prefix", "Single migration"],
		[
			"a prefix without its line break",
			"Answer to ask_user_question call toolu_123: yes",
		],
		[
			"a prefix after other text",
			"Note\nAnswer to ask_user_question call toolu_123:\nyes",
		],
		["a different lead", "Answer to question call toolu_123:\nyes"],
	])("leaves %s alone", (_, text) => {
		expect(parseAsyncAnswer(text)).toBeUndefined();
	});
});

describe("getAsyncAnswerText", () => {
	it("returns the body only for an answer to the same call", () => {
		const answer = `${getAsyncAnswerPrefix("toolu_123")}Single migration`;

		expect(getAsyncAnswerText(answer, "toolu_123")).toBe("Single migration");
		expect(getAsyncAnswerText(answer, "toolu_456")).toBeUndefined();
	});
});
