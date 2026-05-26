import { describe, expect, it } from "vitest";
import { normalizeChatErrorPayload, normalizeChatSendError } from "./chatError";

const apiError = (status: number, data: unknown) => ({
	isAxiosError: true,
	message: `Request failed with status code ${status}`,
	response: {
		status,
		data,
	},
});

describe("normalizeChatErrorPayload", () => {
	it("normalizes streamed chat errors", () => {
		expect(
			normalizeChatErrorPayload({
				message: "  The chat request failed unexpectedly.  ",
				detail: "  stream response: connection reset by peer  ",
				kind: "generic",
				provider: " anthropic ",
				retryable: false,
				status_code: 500,
			}),
		).toEqual({
			message: "The chat request failed unexpectedly.",
			detail: "stream response: connection reset by peer",
			kind: "generic",
			provider: "anthropic",
			retryable: false,
			statusCode: 500,
		});
	});
});

describe("normalizeChatSendError", () => {
	it("uses API response message and detail instead of the axios fallback", () => {
		const error = apiError(500, {
			message: "Failed to create chat message.",
			detail: "insert chat message: pq: relation does not exist",
		});

		expect(normalizeChatSendError(error)).toEqual({
			kind: "generic",
			message: "Failed to create chat message.",
			detail: "insert chat message: pq: relation does not exist",
			statusCode: 500,
		});
	});

	it("keeps usage-limit failures on the usage-limit path", () => {
		const error = apiError(409, {
			message: "Chat usage limit exceeded.",
			spent_micros: 50_000_000,
			limit_micros: 50_000_000,
			resets_at: "2026-07-01T00:00:00Z",
		});

		const normalized = normalizeChatSendError(error);
		expect(normalized?.kind).toBe("usage_limit");
		expect(normalized?.message).toContain("$50.00");
		expect(normalized).not.toHaveProperty("statusCode");
	});

	it("ignores non-API errors", () => {
		expect(normalizeChatSendError(new Error("local failure"))).toBeUndefined();
	});
});
