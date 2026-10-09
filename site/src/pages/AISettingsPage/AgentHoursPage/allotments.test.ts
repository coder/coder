import { describe, expect, it } from "vitest";
import {
	allotmentHours,
	formatAllotmentPercent,
	parseAllotmentPercent,
} from "./allotments";

describe("parseAllotmentPercent", () => {
	it.each([
		["25", 2500],
		["25.5", 2550],
		["0.29", 29],
		["0.01", 1],
		[" 100 ", 10000],
	])("parses %j as %d basis points", (input, bps) => {
		expect(parseAllotmentPercent(input)).toBe(bps);
	});

	it.each(["", "abc", "-5", "1.234", "1e2", ".5", "5."])(
		"rejects %j",
		(input) => {
			expect(parseAllotmentPercent(input)).toBeUndefined();
		},
	);
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
