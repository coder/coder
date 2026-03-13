import { describe, expect, it } from "vitest";
import { formatCostMicros, formatTokenCount } from "./analytics";

describe("formatCostMicros", () => {
	it("formats zero values", () => {
		expect(formatCostMicros(0)).toBe("$0.00");
	});

	it("formats normal values to cents", () => {
		expect(formatCostMicros(1_500_000)).toBe("$1.50");
		expect(formatCostMicros(123_456)).toBe("$0.12");
	});

	it("formats sub-cent values with four decimal places", () => {
		expect(formatCostMicros(500)).toBe("$0.0005");
	});

	it("falls back to zero for invalid numeric values", () => {
		expect(formatCostMicros("abc")).toBe("$0.00");
		expect(formatCostMicros(Number.POSITIVE_INFINITY)).toBe("$0.00");
	});

	it("formats negative values with the minus sign before the dollar sign", () => {
		expect(formatCostMicros(-1_500_000)).toBe("-$1.50");
		expect(formatCostMicros(-500)).toBe("-$0.0005");
	});

	it("avoids confusing four-decimal output when sub-cent values round to one cent", () => {
		expect(formatCostMicros(9_999)).toBe("$0.01");
	});

	it("formats threshold values correctly", () => {
		expect(formatCostMicros(10_000)).toBe("$0.01");
		expect(formatCostMicros(12_345_678)).toBe("$12.35");
	});

	it("formats string micros from generated API types", () => {
		expect(formatCostMicros("1500000")).toBe("$1.50");
	});
});

describe("formatTokenCount", () => {
	it("formats zero values", () => {
		expect(formatTokenCount(0)).toBe("0");
	});

	it("formats normal values with locale separators", () => {
		expect(formatTokenCount(999)).toBe("999");
		expect(formatTokenCount(1_234)).toBe("1,234");
		expect(formatTokenCount(999_999)).toBe("999,999");
	});

	it("formats large values in millions", () => {
		expect(formatTokenCount(1_000_000)).toBe("1M");
		expect(formatTokenCount(1_500_000)).toBe("1.5M");
		expect(formatTokenCount(2_000_000)).toBe("2M");
	});

	it("rounds million values to one decimal place when needed", () => {
		expect(formatTokenCount(1_250_000)).toBe("1.3M");
	});
});
