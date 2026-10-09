import { describe, expect, it } from "vitest";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
	getPersistedDetailError,
} from "./chatError";

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

describe("getPersistedDetailError", () => {
	it("returns the persisted error only while the chat is in error", () => {
		const cachedError: ChatDetailError = {
			kind: "generic",
			message: "turn failed",
		};

		// A pending edit shows its turn as running, which hides the error of
		// the turn it replaces.
		expect(
			getPersistedDetailError({
				chatStatus: "running",
				chatRecord: undefined,
				cachedError,
			}),
		).toBeUndefined();
		expect(
			getPersistedDetailError({
				chatStatus: "error",
				chatRecord: undefined,
				cachedError,
			}),
		).toEqual(cachedError);
	});
});
