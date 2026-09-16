import { describe, expect, it } from "vitest";
import { type ChatDetailError, chatDetailErrorsEqual } from "./chatError";

describe("chatDetailErrorsEqual", () => {
	it("compares matching errors by value", () => {
		const left: ChatDetailError = {
			kind: "rate_limit",
			message: "Slow down.",
			provider: "anthropic",
			retryable: true,
			statusCode: 429,
		};

		expect(chatDetailErrorsEqual(left, { ...left })).toBe(true);
	});

	it("treats missing and mismatched errors as different", () => {
		const error: ChatDetailError = {
			kind: "generic",
			message: "Provider request failed.",
		};

		expect(chatDetailErrorsEqual(error, null)).toBe(false);
		expect(chatDetailErrorsEqual(error, { ...error, statusCode: 500 })).toBe(
			false,
		);
		expect(
			chatDetailErrorsEqual(error, { ...error, detail: "Bad image." }),
		).toBe(false);
	});
});
