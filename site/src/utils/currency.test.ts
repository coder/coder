import { describe, expect, it } from "vitest";

import {
	dollarsToMicros,
	formatCostMicros,
	isPositiveFiniteDollarAmount,
	MICROS_PER_DOLLAR,
	microsToDollars,
} from "./currency";

describe("MICROS_PER_DOLLAR", () => {
	it("matches the expected million-micros constant", () => {
		expect(MICROS_PER_DOLLAR).toBe(1_000_000);
	});
});

describe("microsToDollars", () => {
	it("converts micros to dollars without rounding", () => {
		expect(microsToDollars(1_500_000)).toBe(1.5);
		expect(microsToDollars(500)).toBe(0.0005);
		expect(microsToDollars(0)).toBe(0);
		expect(microsToDollars(125_000_000)).toBe(125);
	});
});

describe("dollarsToMicros", () => {
	it("converts string dollar amounts to micros", () => {
		expect(dollarsToMicros("12.34")).toBe(12_340_000);
		expect(dollarsToMicros("0.000001")).toBe(1);
	});

	it("returns zero for blank or non-finite inputs", () => {
		expect(dollarsToMicros("")).toBe(0);
		expect(dollarsToMicros("NaN")).toBe(0);
		expect(dollarsToMicros("Infinity")).toBe(0);
	});

	it("returns zero for negative or sub-micro inputs", () => {
		expect(dollarsToMicros("-1")).toBe(0);
		expect(dollarsToMicros("1e-10")).toBe(0);
	});

	it("accepts number inputs", () => {
		expect(dollarsToMicros(12.34)).toBe(12_340_000);
	});
});

describe("isPositiveFiniteDollarAmount", () => {
	it("accepts positive finite dollar amounts that round to at least one micro", () => {
		expect(isPositiveFiniteDollarAmount("12.34")).toBe(true);
		expect(isPositiveFiniteDollarAmount("1e2")).toBe(true);
	});

	it("rejects blank, invalid, non-positive, and sub-micro values", () => {
		expect(isPositiveFiniteDollarAmount("")).toBe(false);
		expect(isPositiveFiniteDollarAmount("0")).toBe(false);
		expect(isPositiveFiniteDollarAmount("-1")).toBe(false);
		expect(isPositiveFiniteDollarAmount("abc")).toBe(false);
		expect(isPositiveFiniteDollarAmount("1e-10")).toBe(false);
		expect(isPositiveFiniteDollarAmount("1e309")).toBe(false);
	});
});

describe("formatCostMicros", () => {
	it("formats zero values", () => {
		expect(formatCostMicros(0)).toBe("$0.00");
	});

	it("formats normal dollar values with two decimals", () => {
		expect(formatCostMicros(1_500_000)).toBe("$1.50");
		expect(formatCostMicros(123_456)).toBe("$0.12");
	});

	it("formats sub-cent values with four decimals", () => {
		expect(formatCostMicros(500)).toBe("$0.0005");
	});

	it("falls back to zero for invalid numeric values", () => {
		expect(formatCostMicros("abc")).toBe("$0.00");
		expect(formatCostMicros(Number.POSITIVE_INFINITY)).toBe("$0.00");
	});

	it("formats negative values with the sign before the dollar symbol", () => {
		expect(formatCostMicros(-1_500_000)).toBe("-$1.50");
		expect(formatCostMicros(-500)).toBe("-$0.0005");
	});

	it("uses the normal currency formatter when sub-cent values round to one cent", () => {
		expect(formatCostMicros(9_999)).toBe("$0.01");
	});

	it("formats threshold, rounded, string, and grouped values correctly", () => {
		expect(formatCostMicros(10_000)).toBe("$0.01");
		expect(formatCostMicros(12_345_678)).toBe("$12.35");
		expect(formatCostMicros("1500000")).toBe("$1.50");
		expect(formatCostMicros(1_234_560_000)).toBe("$1,234.56");
	});
});
