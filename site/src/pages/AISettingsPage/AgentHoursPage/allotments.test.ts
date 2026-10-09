import { describe, expect, it } from "vitest";
import {
	allotmentHours,
	formatAllotmentPercent,
	formatHours,
	parseAllotmentPercent,
} from "./allotments";

describe("parseAllotmentPercent", () => {
	it.each([
		["25", 2500],
		["25.5", 2550],
		["0.29", 29],
		["0.01", 1],
		[" 100 ", 10000],
		[".5", 50],
		["5.", 500],
	])("parses %j as %d basis points", (input, bps) => {
		expect(parseAllotmentPercent(input)).toBe(bps);
	});

	it.each(["", ".", "abc", "-5", "1.234", "1e2"])("rejects %j", (input) => {
		expect(parseAllotmentPercent(input)).toBeUndefined();
	});
});

describe("formatAllotmentPercent", () => {
	it.each([
		[2550, "25.5%"],
		[1, "0.01%"],
		[10000, "100%"],
	])("formats %d as %s", (bps, text) => {
		expect(formatAllotmentPercent(bps)).toBe(text);
	});
});

describe("allotmentHours", () => {
	it("converts a share of a finite pool to hours", () => {
		expect(allotmentHours(2500, 1000)).toBe(250);
	});

	it("has no hours for an unknown pool", () => {
		expect(allotmentHours(2500, undefined)).toBeUndefined();
	});
});

describe("formatHours", () => {
	it("keeps the hours of the smallest share of a small pool", () => {
		// 0.01% of 600 hours.
		expect(formatHours(0.06)).toBe("0.06 hours");
	});

	it("does not round a non-zero share down to zero hours", () => {
		// 0.01% of 0.01% of 1,000 hours.
		expect(formatHours(0.00001)).toBe("< 0.01 hours");
	});

	it("formats no hours as zero", () => {
		expect(formatHours(0)).toBe("0 hours");
	});
});
