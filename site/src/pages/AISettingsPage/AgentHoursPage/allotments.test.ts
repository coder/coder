import { describe, expect, it } from "vitest";
import {
	allotmentHours,
	allotmentTargetLabel,
	formatAllotmentPercent,
	formatHours,
	notAttributedMs,
	parseAllotmentPercent,
	usageWithoutAllotment,
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

describe("usageWithoutAllotment", () => {
	const mockEngineering = {
		id: "1",
		name: "eng",
		display_name: "Engineering",
		usedMs: 3_600_000,
	};
	const mockPlatform = {
		id: "2",
		name: "platform",
		display_name: "Platform",
		usedMs: 7_200_000,
	};
	const mockIdle = { id: "3", name: "idle", display_name: "", usedMs: 0 };
	const mockDeleted = { id: "4", name: "", display_name: "", usedMs: 60_000 };
	const used = [mockEngineering, mockPlatform, mockIdle, mockDeleted];

	it("lists used targets without an allotment and labels deleted ones", () => {
		expect(
			usageWithoutAllotment(used, [{ id: mockPlatform.id }], {
				targets: used,
				deletedLabel: "Deleted group",
				href: (name) => `/groups/${name}`,
			}),
		).toEqual([
			{
				id: mockEngineering.id,
				name: "Engineering",
				usedMs: 3_600_000,
				href: "/groups/eng",
			},
			{ id: mockDeleted.id, name: "Deleted group", usedMs: 60_000 },
		]);
	});

	it("tells apart targets that share a display name", () => {
		const mockNamesake = { ...mockPlatform, display_name: "Engineering" };
		expect(
			usageWithoutAllotment([mockNamesake], [], {
				targets: [mockEngineering, mockNamesake],
				deletedLabel: "Deleted organization",
			}),
		).toEqual([
			{
				id: mockNamesake.id,
				name: "Engineering (platform)",
				usedMs: 7_200_000,
				href: undefined,
			},
		]);
	});
});

describe("notAttributedMs", () => {
	it("is the share of the total that no organization accounts for", () => {
		expect(
			notAttributedMs(10_000, [{ used_ms: 6_000 }, { used_ms: 1_000 }]),
		).toBe(3_000);
	});

	it("is zero when the organizations add up to more than the total", () => {
		expect(notAttributedMs(10_000, [{ used_ms: 10_500 }])).toBe(0);
	});
});
