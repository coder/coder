import { describe, expect, it } from "vitest";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
	isChatDiffReauthRequired,
} from "./chatError";

describe("isChatDiffReauthRequired", () => {
	it("accepts a reauth body with its provider fields", () => {
		expect(isChatDiffReauthRequired(MockChatDiffReauthRequired)).toBe(true);
	});

	it("rejects bodies with another kind or mistyped fields", () => {
		expect(
			isChatDiffReauthRequired({
				...MockChatDiffReauthRequired,
				kind: "generic",
			}),
		).toBe(false);
		expect(
			isChatDiffReauthRequired({
				...MockChatDiffReauthRequired,
				can_reconnect: "true",
			}),
		).toBe(false);
		expect(isChatDiffReauthRequired(undefined)).toBe(false);
	});
});

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
