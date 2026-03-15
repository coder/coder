import {
	dollarsToMicros,
	isPositiveFiniteDollarAmount,
	microsToDollars,
	normalizeChatUsageLimitPeriod,
} from "./limitsFormLogic";

describe("limitsFormLogic", () => {
	describe("microsToDollars", () => {
		it("converts micros to dollars", () => {
			expect(microsToDollars(125_000_000)).toBe(125);
		});
	});

	describe("dollarsToMicros", () => {
		it("converts dollars to micros", () => {
			expect(dollarsToMicros("12.34")).toBe(12_340_000);
		});

		it("returns zero for non-finite or empty inputs", () => {
			expect(dollarsToMicros("NaN")).toBe(0);
			expect(dollarsToMicros("Infinity")).toBe(0);
			expect(dollarsToMicros("")).toBe(0);
		});
	});

	describe("isPositiveFiniteDollarAmount", () => {
		it("accepts positive finite amounts", () => {
			expect(isPositiveFiniteDollarAmount("12.34")).toBe(true);
			expect(isPositiveFiniteDollarAmount("1e2")).toBe(true);
		});

		it("rejects blank, non-positive, NaN, non-finite, and sub-microcent amounts", () => {
			expect(isPositiveFiniteDollarAmount("")).toBe(false);
			expect(isPositiveFiniteDollarAmount("0")).toBe(false);
			expect(isPositiveFiniteDollarAmount("-1")).toBe(false);
			expect(isPositiveFiniteDollarAmount("abc")).toBe(false);
			expect(isPositiveFiniteDollarAmount("1e-10")).toBe(false);
			expect(isPositiveFiniteDollarAmount("1e309")).toBe(false);
		});
	});

	describe("normalizeChatUsageLimitPeriod", () => {
		it("defaults invalid periods to month", () => {
			expect(normalizeChatUsageLimitPeriod("year")).toBe("month");
			expect(normalizeChatUsageLimitPeriod(undefined)).toBe("month");
		});
	});
});
