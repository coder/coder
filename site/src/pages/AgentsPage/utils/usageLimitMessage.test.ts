import {
	type ChatDetailError,
	chatDetailErrorsEqual,
	formatUsageLimitMessage,
	isUsageLimitData,
} from "./usageLimitMessage";

describe("formatUsageLimitMessage", () => {
	it("formats a full structured message", () => {
		const result = formatUsageLimitMessage({
			spent_micros: 900_000,
			limit_micros: 500_000,
			resets_at: "2026-03-16T00:00:00Z",
		});
		expect(result).toContain("$0.90");
		expect(result).toContain("$0.50");
		expect(result).toContain("Mar");
		expect(result).toContain("2026");
	});

	it("returns fallback when fields are missing", () => {
		expect(formatUsageLimitMessage({})).toBe(
			"Your usage limit has been reached.",
		);
		expect(formatUsageLimitMessage({ spent_micros: 100 })).toBe(
			"Your usage limit has been reached.",
		);
	});

	it("returns fallback for custom fallback message", () => {
		expect(formatUsageLimitMessage({}, "Custom fallback.")).toBe(
			"Custom fallback.",
		);
	});

	it("formats zero-value amounts", () => {
		const result = formatUsageLimitMessage({
			spent_micros: 0,
			limit_micros: 0,
			resets_at: "2026-03-16T00:00:00Z",
		});
		expect(result).toContain("$0.00");
	});

	it("formats high-value amounts with locale grouping", () => {
		const result = formatUsageLimitMessage({
			spent_micros: 1_234_560_000,
			limit_micros: 5_000_000_000,
			resets_at: "2026-03-16T00:00:00Z",
		});
		expect(result).toContain("$1,234.56");
		expect(result).toContain("$5,000.00");
	});

	it("formats sub-cent values with four decimal places", () => {
		const result = formatUsageLimitMessage({
			spent_micros: 500,
			limit_micros: 1_000,
			resets_at: "2026-03-16T00:00:00Z",
		});
		expect(result).toContain("$0.0005");
		expect(result).toContain("$0.0010");
	});

	it("handles invalid resets_at gracefully", () => {
		const result = formatUsageLimitMessage({
			spent_micros: 900_000,
			limit_micros: 500_000,
			resets_at: "not-a-date",
		});
		expect(result).toContain("$0.90");
		expect(result).toContain("$0.50");
		expect(result).not.toContain("Resets");
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
	});
});

describe("isUsageLimitData", () => {
	it("accepts a fully populated valid payload", () => {
		const error: ChatDetailError = {
			message: "Your usage limit has been reached.",
			kind: "usage-limit",
		};

		expect(error.kind).toBe("usage-limit");
		expect(
			isUsageLimitData({
				spent_micros: 900_000,
				limit_micros: 500_000,
				resets_at: "2026-03-16T00:00:00Z",
			}),
		).toBe(true);
	});

	it("rejects null", () => {
		expect(isUsageLimitData(null)).toBe(false);
	});

	it("rejects undefined", () => {
		expect(isUsageLimitData(undefined)).toBe(false);
	});

	it("rejects an empty object (missing all fields)", () => {
		expect(isUsageLimitData({})).toBe(false);
	});

	it("rejects when spent_micros is missing", () => {
		expect(
			isUsageLimitData({
				limit_micros: 500_000,
				resets_at: "2026-03-16T00:00:00Z",
			}),
		).toBe(false);
	});

	it("rejects when limit_micros is missing", () => {
		expect(
			isUsageLimitData({
				spent_micros: 900_000,
				resets_at: "2026-03-16T00:00:00Z",
			}),
		).toBe(false);
	});

	it("rejects when resets_at is missing", () => {
		expect(
			isUsageLimitData({ spent_micros: 900_000, limit_micros: 500_000 }),
		).toBe(false);
	});

	it("rejects wrong field types (string for spent_micros)", () => {
		expect(
			isUsageLimitData({
				spent_micros: "900000",
				limit_micros: 500_000,
				resets_at: "2026-03-16T00:00:00Z",
			}),
		).toBe(false);
	});

	it("accepts payload with extra fields", () => {
		expect(
			isUsageLimitData({
				spent_micros: 900_000,
				limit_micros: 500_000,
				resets_at: "2026-03-16T00:00:00Z",
				extra_field: "ignored",
			}),
		).toBe(true);
	});
});
