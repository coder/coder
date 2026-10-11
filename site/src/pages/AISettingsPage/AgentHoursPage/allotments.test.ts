import { describe, expect, it } from "vitest";
import {
	allotmentHours,
	allotmentTargetLabel,
	formatAllotmentPercent,
	formatHours,
	formatUsedHours,
	parseAllotmentPercent,
	remainderHours,
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
		expect(parseAllotmentPercent(input)).toEqual({ bps });
	});

	it.each([
		["", "not-a-number"],
		[".", "not-a-number"],
		["abc", "not-a-number"],
		["-5", "not-a-number"],
		["1e2", "not-a-number"],
		["1.234", "too-many-decimals"],
	])("rejects %j as %s", (input, error) => {
		expect(parseAllotmentPercent(input)).toEqual({ error });
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

describe("formatUsedHours", () => {
	it.each([
		[0, "0.0"],
		[359_999, "0.0"],
		[360_000, "0.1"],
		[4_499_999_999, "1,249.9"],
		[Number.NaN, "0.0"],
	])("formats %d ms as %s hours", (ms, text) => {
		expect(formatUsedHours(ms)).toBe(text);
	});
});

describe("remainderHours", () => {
	it("is the share of a finite pool that no allotment claims", () => {
		expect(remainderHours(6000, 1000)).toBe(400);
	});

	it("is zero for a pool allotted beyond 100%", () => {
		expect(remainderHours(10500, 1000)).toBe(0);
	});

	it("has no hours for an unknown pool", () => {
		expect(remainderHours(6000, undefined)).toBeUndefined();
	});
});

describe("allotmentTargetLabel", () => {
	const mockEngineering = { id: "1", name: "eng", display_name: "Engineering" };
	const mockPlatform = {
		id: "2",
		name: "platform",
		display_name: "Engineering",
	};
	const mockNameOnly = { id: "3", name: "Engineering", display_name: "" };

	it.each([
		{
			target: mockEngineering,
			targets: [mockEngineering],
			label: "Engineering",
		},
		{
			target: mockPlatform,
			targets: [mockEngineering, mockPlatform],
			label: "Engineering (platform)",
		},
		{
			target: mockNameOnly,
			targets: [mockEngineering, mockNameOnly],
			label: "Engineering",
		},
	])("labels $target.name as $label", ({ target, targets, label }) => {
		expect(allotmentTargetLabel(target, targets)).toBe(label);
	});
});
