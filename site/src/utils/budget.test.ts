import { describe, expect, it } from "vitest";
import {
	clampPercentage,
	formatSpendPeriodLabel,
	getSeverity,
	MIN_PROJECTION_ELAPSED_MS,
	projectPeriodSpendMicros,
	usageProgressPercentage,
} from "./budget";

describe("formatSpendPeriodLabel", () => {
	it("renders API timestamps in UTC with the year on the exclusive end", () => {
		expect(
			formatSpendPeriodLabel("2026-06-01T00:00:00Z", "2026-07-01T00:00:00Z"),
		).toBe("June 1 - July 1, 2026");
	});

	it("uses the end year when the window crosses into the next year", () => {
		expect(
			formatSpendPeriodLabel("2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z"),
		).toBe("December 1 - January 1, 2027");
	});
});

describe("getSeverity", () => {
	it("returns normal below the warning threshold", () => {
		expect(getSeverity(0, 50)).toBe("normal");
		expect(getSeverity(42, 50)).toBe("normal");
	});

	it("returns warning at or above 85% of the budget", () => {
		expect(getSeverity(42.5, 50)).toBe("warning");
		expect(getSeverity(46, 50)).toBe("warning");
	});

	it("returns exceeded once usage meets or passes the budget", () => {
		expect(getSeverity(50, 50)).toBe("exceeded");
		expect(getSeverity(75, 50)).toBe("exceeded");
	});

	it("treats a zero budget as exceeded once anything is used", () => {
		expect(getSeverity(0, 0)).toBe("normal");
		expect(getSeverity(5, 0)).toBe("exceeded");
	});

	it("returns normal for non-finite or negative inputs", () => {
		expect(getSeverity(Number.NaN, 50)).toBe("normal");
		expect(getSeverity(10, Number.POSITIVE_INFINITY)).toBe("normal");
		expect(getSeverity(10, -50)).toBe("normal");
	});
});

describe("usageProgressPercentage", () => {
	it("returns the usage percentage clamped from 0 to 100", () => {
		expect(usageProgressPercentage(25, 100)).toBe(25);
		expect(usageProgressPercentage(125, 100)).toBe(100);
		expect(usageProgressPercentage(-25, 100)).toBe(0);
	});

	it("handles zero budgets and invalid inputs", () => {
		expect(usageProgressPercentage(0, 0)).toBe(0);
		expect(usageProgressPercentage(1, 0)).toBe(100);
		expect(usageProgressPercentage(Number.NaN, 100)).toBe(0);
		expect(usageProgressPercentage(1, Number.POSITIVE_INFINITY)).toBe(0);
		expect(usageProgressPercentage(1, -100)).toBe(0);
	});
});

describe("projectPeriodSpendMicros", () => {
	const periodStart = "2026-07-01T00:00:00Z";
	const periodEnd = "2026-08-01T00:00:00Z";
	const startMs = Date.parse(periodStart);
	const dayMs = 24 * 60 * 60 * 1000;

	it("scales spend so far across the full period", () => {
		// 10 of 31 days elapsed: $12.50 * 31 / 10 = $38.75.
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 12_500_000,
				periodStart,
				periodEnd,
				nowMs: startMs + 10 * dayMs,
			}),
		).toBe(38_750_000);
	});

	it("rounds to whole micros", () => {
		// 6 of 31 days elapsed: 1,000,000 * 31 / 6 = 5,166,666.67, which
		// distinguishes rounding from truncation.
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 1_000_000,
				periodStart,
				periodEnd,
				nowMs: startMs + 6 * dayMs,
			}),
		).toBe(5_166_667);
	});

	it("hides the projection until the minimum elapsed time", () => {
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd,
				nowMs: startMs + MIN_PROJECTION_ELAPSED_MS - 1,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd,
				nowMs: startMs + MIN_PROJECTION_ELAPSED_MS,
			}),
		).toBe(155_000_000);
	});

	it("clamps elapsed time to the period", () => {
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd,
				nowMs: Date.parse(periodEnd) + 5 * dayMs,
			}),
		).toBe(5_000_000);
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd,
				nowMs: startMs - dayMs,
			}),
		).toBeUndefined();
	});

	it("returns undefined without spend or with invalid inputs", () => {
		const nowMs = startMs + 10 * dayMs;
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 0,
				periodStart,
				periodEnd,
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: -5_000_000,
				periodStart,
				periodEnd,
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: Number.NaN,
				periodStart,
				periodEnd,
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd,
				nowMs: Number.NaN,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart: "not a date",
				periodEnd,
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd: "not a date",
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart,
				periodEnd: periodStart,
				nowMs,
			}),
		).toBeUndefined();
		expect(
			projectPeriodSpendMicros({
				currentSpendMicros: 5_000_000,
				periodStart: periodEnd,
				periodEnd: periodStart,
				nowMs,
			}),
		).toBeUndefined();
	});
});

describe("clampPercentage", () => {
	it("clamps percentages from 0 to 100", () => {
		expect(clampPercentage(-1)).toBe(0);
		expect(clampPercentage(50)).toBe(50);
		expect(clampPercentage(101)).toBe(100);
		expect(clampPercentage(Number.NaN)).toBe(0);
	});
});
