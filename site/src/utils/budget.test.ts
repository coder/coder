import { describe, expect, it } from "vitest";
import {
	clampPercentage,
	getSeverity,
	usageProgressPercentage,
} from "./budget";

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

describe("clampPercentage", () => {
	it("clamps percentages from 0 to 100", () => {
		expect(clampPercentage(-1)).toBe(0);
		expect(clampPercentage(50)).toBe(50);
		expect(clampPercentage(101)).toBe(100);
		expect(clampPercentage(Number.NaN)).toBe(0);
	});
});
