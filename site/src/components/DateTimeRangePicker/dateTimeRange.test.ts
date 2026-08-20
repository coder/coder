import { describe, expect, it } from "vitest";
import {
	combineDateTime,
	DEFAULT_QUICK_PRESETS,
	formatCustomLabel,
	parseClockTime,
	toClockFields,
} from "./dateTimeRange";

const clock = (text: string) => {
	const parsed = parseClockTime(text);
	if (parsed === null) {
		throw new Error(`expected "${text}" to parse`);
	}
	return parsed;
};

describe("parseClockTime", () => {
	it("parses full and minute-only times", () => {
		expect(parseClockTime("09:30:15")).toEqual({
			hours: 9,
			minutes: 30,
			seconds: 15,
		});
		expect(parseClockTime("12:00:00")).toEqual({
			hours: 12,
			minutes: 0,
			seconds: 0,
		});
		expect(parseClockTime("09:30")).toEqual({
			hours: 9,
			minutes: 30,
			seconds: 0,
		});
	});

	it("accepts single-digit hours and surrounding whitespace", () => {
		expect(parseClockTime("9:05")).toEqual({
			hours: 9,
			minutes: 5,
			seconds: 0,
		});
		expect(parseClockTime(" 11:15:30 ")).toEqual({
			hours: 11,
			minutes: 15,
			seconds: 30,
		});
	});

	it("enforces the 1-12 hour range of a 12-hour clock", () => {
		expect(parseClockTime("1:00")).not.toBeNull();
		expect(parseClockTime("12:59:59")).not.toBeNull();
		expect(parseClockTime("0:30")).toBeNull();
		expect(parseClockTime("13:00")).toBeNull();
		expect(parseClockTime("99:99")).toBeNull();
	});

	it("rejects out-of-range minutes and seconds", () => {
		expect(parseClockTime("09:60")).toBeNull();
		expect(parseClockTime("09:30:60")).toBeNull();
	});

	it("rejects unknown shapes", () => {
		expect(parseClockTime("")).toBeNull();
		expect(parseClockTime("09")).toBeNull();
		expect(parseClockTime("09:5")).toBeNull();
		expect(parseClockTime("09:30:15:00")).toBeNull();
		expect(parseClockTime("9:30 AM")).toBeNull();
		expect(parseClockTime("noon")).toBeNull();
	});
});

describe("combineDateTime", () => {
	const day = new Date(2026, 3, 10, 17, 45, 33);

	it("maps 12 AM to midnight and 12 PM to noon", () => {
		expect(combineDateTime(day, clock("12:00:00"), "AM")).toEqual(
			new Date(2026, 3, 10, 0, 0, 0),
		);
		expect(combineDateTime(day, clock("12:00:00"), "PM")).toEqual(
			new Date(2026, 3, 10, 12, 0, 0),
		);
	});

	it("maps other hours by meridiem", () => {
		expect(combineDateTime(day, clock("9:30:15"), "AM")).toEqual(
			new Date(2026, 3, 10, 9, 30, 15),
		);
		expect(combineDateTime(day, clock("9:30:15"), "PM")).toEqual(
			new Date(2026, 3, 10, 21, 30, 15),
		);
	});

	it("keeps the calendar day and discards the source time", () => {
		expect(combineDateTime(day, clock("1:02:03"), "AM")).toEqual(
			new Date(2026, 3, 10, 1, 2, 3),
		);
	});
});

describe("toClockFields", () => {
	it("splits midnight and noon into 12-hour fields", () => {
		expect(toClockFields(new Date(2026, 3, 10, 0, 0, 0))).toEqual({
			time: "12:00:00",
			meridiem: "AM",
		});
		expect(toClockFields(new Date(2026, 3, 10, 12, 0, 0))).toEqual({
			time: "12:00:00",
			meridiem: "PM",
		});
	});

	it("splits arbitrary times", () => {
		expect(toClockFields(new Date(2026, 3, 10, 9, 5, 7))).toEqual({
			time: "09:05:07",
			meridiem: "AM",
		});
		expect(toClockFields(new Date(2026, 3, 10, 21, 30, 0))).toEqual({
			time: "09:30:00",
			meridiem: "PM",
		});
	});

	it("round-trips through combineDateTime", () => {
		const original = new Date(2026, 3, 10, 21, 30, 45);
		const { time, meridiem } = toClockFields(original);
		expect(combineDateTime(original, clock(time), meridiem)).toEqual(original);
	});
});

describe("formatCustomLabel", () => {
	it("shows times for intra-day ranges", () => {
		expect(
			formatCustomLabel(
				new Date(2026, 3, 12, 9, 0, 0),
				new Date(2026, 3, 12, 11, 30, 0),
			),
		).toBe("April 12, 9:00 AM - 11:30 AM");
	});

	it("collapses ranges within one month to a day span", () => {
		expect(
			formatCustomLabel(new Date(2026, 3, 10), new Date(2026, 3, 16)),
		).toBe("April 10-16");
	});

	it("spells out both months within one year", () => {
		expect(formatCustomLabel(new Date(2026, 2, 28), new Date(2026, 3, 2))).toBe(
			"Mar 28 - Apr 2",
		);
	});

	it("includes years when they differ", () => {
		expect(
			formatCustomLabel(new Date(2025, 11, 30), new Date(2026, 0, 2)),
		).toBe("Dec 30, 2025 - Jan 2, 2026");
	});
});

describe("DEFAULT_QUICK_PRESETS", () => {
	const now = new Date(2026, 3, 16, 10, 30, 0);
	const rangeFor = (id: string) => {
		const preset = DEFAULT_QUICK_PRESETS.find((p) => p.id === id);
		if (!preset) {
			throw new Error(`missing preset ${id}`);
		}
		return preset.range(now);
	};

	it("resolves last_15m and last_1h relative to now", () => {
		expect(rangeFor("last_15m")).toEqual({
			start: new Date(2026, 3, 16, 10, 15, 0),
			end: now,
		});
		expect(rangeFor("last_1h")).toEqual({
			start: new Date(2026, 3, 16, 9, 30, 0),
			end: now,
		});
	});

	it("resolves today from local midnight", () => {
		expect(rangeFor("today")).toEqual({
			start: new Date(2026, 3, 16, 0, 0, 0),
			end: now,
		});
	});

	it("resolves this_week from the start of the week", () => {
		// April 16, 2026 is a Thursday; the week starts Sunday, April 12.
		expect(rangeFor("this_week")).toEqual({
			start: new Date(2026, 3, 12, 0, 0, 0),
			end: now,
		});
	});
});
