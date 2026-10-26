import { describe, expect, it } from "vitest";
import { getAsyncAnswerPrefix, parseAsyncAnswer } from "./asyncAnswer";

describe("parseAsyncAnswer", () => {
	it("splits an answer into the call it names and its body", () => {
		expect(
			parseAsyncAnswer(`${getAsyncAnswerPrefix("toolu_123")}1. Scope: Backend`),
		).toEqual({ toolCallId: "toolu_123", body: "1. Scope: Backend" });
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
