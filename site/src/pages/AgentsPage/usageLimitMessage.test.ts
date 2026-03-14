import { formatUsageLimitMessage } from "./usageLimitMessage";

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
