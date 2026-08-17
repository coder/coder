import { describe, expect, it } from "vitest";
import {
	defaultTimeRange,
	formatTimeExpression,
	formatTriggerLabel,
	parseTimeExpression,
	parseTimeRange,
	setTimeRangeInQuery,
	type TimeRange,
	toRFC3339,
	withDefaultTimeRange,
} from "./timeRange";

const now = new Date(2026, 7, 13, 15, 0, 0);

describe("parseTimeExpression", () => {
	it("parses now case-insensitively", () => {
		expect(parseTimeExpression("now", now)).toEqual(now);
		expect(parseTimeExpression("Now", now)).toEqual(now);
		expect(parseTimeExpression(" NOW ", now)).toEqual(now);
	});

	it("parses clock times against the current day", () => {
		expect(parseTimeExpression("15:43", now)).toEqual(
			new Date(2026, 7, 13, 15, 43, 0),
		);
		expect(parseTimeExpression("9:05:09", now)).toEqual(
			new Date(2026, 7, 13, 9, 5, 9),
		);
	});

	it("defaults a bare date to midnight", () => {
		expect(parseTimeExpression("2026-08-13", now)).toEqual(
			new Date(2026, 7, 13),
		);
	});

	it("parses date and time together", () => {
		expect(parseTimeExpression("2026-08-13 11:43", now)).toEqual(
			new Date(2026, 7, 13, 11, 43, 0),
		);
		expect(parseTimeExpression("2026-08-13 11:43:21", now)).toEqual(
			new Date(2026, 7, 13, 11, 43, 21),
		);
	});

	it("rejects out-of-range clocks and dates", () => {
		expect(parseTimeExpression("23:59:99", now)).toBeNull();
		expect(parseTimeExpression("24:00", now)).toBeNull();
		expect(parseTimeExpression("2026-02-30", now)).toBeNull();
		expect(parseTimeExpression("2026-13-01", now)).toBeNull();
		expect(parseTimeExpression("2026-08-13 23:59:99", now)).toBeNull();
	});

	it("rejects unknown shapes", () => {
		expect(parseTimeExpression("", now)).toBeNull();
		expect(parseTimeExpression("30d", now)).toBeNull();
		expect(parseTimeExpression("13/08/2026", now)).toBeNull();
		expect(parseTimeExpression("2026-08-13T11:43", now)).toBeNull();
	});
});

describe("formatTimeExpression", () => {
	it("round-trips through parseTimeExpression", () => {
		const date = new Date(2026, 7, 13, 7, 23, 0);
		expect(parseTimeExpression(formatTimeExpression(date), now)).toEqual(date);
	});

	it("pads single-digit fields", () => {
		expect(formatTimeExpression(new Date(2026, 0, 2, 3, 4, 5))).toBe(
			"2026-01-02 03:04:05",
		);
	});
});

describe("toRFC3339", () => {
	it("serializes in UTC with second precision", () => {
		expect(toRFC3339(new Date(Date.UTC(2026, 7, 13, 10, 0, 0, 500)))).toBe(
			"2026-08-13T10:00:00Z",
		);
	});
});

describe("defaultTimeRange", () => {
	it("spans the 24 hours ending at now", () => {
		const range = defaultTimeRange(now);
		expect(range.startedBefore).toEqual(now);
		expect(range.startedAfter).toEqual(
			new Date(now.getTime() - 24 * 60 * 60 * 1000),
		);
	});
});

describe("formatTriggerLabel", () => {
	it("collapses a single day", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 3, 10, 7, 23, 0),
					startedBefore: new Date(2026, 3, 10, 9, 30, 0),
				},
				now,
			),
		).toBe("Apr 10");
	});

	it("labels a range ending today against now", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 7, 11, 23, 59, 59),
					startedBefore: new Date(2026, 7, 13, 10, 0, 0),
				},
				now,
			),
		).toBe("Aug 11 - Today");
	});

	it("shortens ranges within one month", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 3, 17),
					startedBefore: new Date(2026, 3, 19),
				},
				now,
			),
		).toBe("Apr 17 - 19");
	});

	it("falls back to a full range across months", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 2, 30),
					startedBefore: new Date(2026, 3, 2),
				},
				now,
			),
		).toBe("Mar 30 - Apr 2");
	});
});

describe("withDefaultTimeRange", () => {
	const range: TimeRange = {
		startedAfter: new Date(Date.UTC(2026, 7, 12, 15, 0, 0)),
		startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
	};

	it("appends the default range to an empty query", () => {
		expect(withDefaultTimeRange("", range)).toBe(
			'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"',
		);
	});

	it("appends the default range alongside other filters", () => {
		expect(withDefaultTimeRange("initiator:me", range)).toBe(
			'initiator:me started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"',
		);
	});

	it("leaves a query with an explicit time range untouched", () => {
		const afterOnly = 'started_after:"2026-08-01T00:00:00Z"';
		expect(withDefaultTimeRange(afterOnly, range)).toBe(afterOnly);

		const beforeOnly = 'started_before:"2026-08-01T00:00:00Z"';
		expect(withDefaultTimeRange(beforeOnly, range)).toBe(beforeOnly);
	});
});

describe("setTimeRangeInQuery", () => {
	const range: TimeRange = {
		startedAfter: new Date(Date.UTC(2026, 7, 12, 15, 0, 0)),
		startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
	};

	it("preserves other filters and replaces the time range", () => {
		expect(
			setTimeRangeInQuery(
				{
					initiator: "me",
					started_after: "2026-01-01T00:00:00Z",
					started_before: "2026-01-02T00:00:00Z",
				},
				range,
			),
		).toBe(
			'initiator:me started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"',
		);
	});

	it("quotes values containing spaces", () => {
		expect(setTimeRangeInQuery({ session_id: "abc def" }, range)).toContain(
			'session_id:"abc def"',
		);
	});

	it("drops empty values", () => {
		expect(setTimeRangeInQuery({ initiator: undefined }, range)).toBe(
			'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"',
		);
	});
});

describe("parseTimeRange", () => {
	it("parses an explicit range", () => {
		expect(
			parseTimeRange({
				started_after: "2026-08-12T15:00:00Z",
				started_before: "2026-08-13T15:00:00Z",
			}),
		).toEqual({
			startedAfter: new Date(Date.UTC(2026, 7, 12, 15, 0, 0)),
			startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
		});
	});

	it("returns null when either bound is missing", () => {
		expect(parseTimeRange({})).toBeNull();
		expect(
			parseTimeRange({ started_after: "2026-08-12T15:00:00Z" }),
		).toBeNull();
		expect(
			parseTimeRange({ started_before: "2026-08-13T15:00:00Z" }),
		).toBeNull();
	});

	it("returns null for malformed bounds", () => {
		expect(
			parseTimeRange({
				started_after: "bogus",
				started_before: "2026-08-13T15:00:00Z",
			}),
		).toBeNull();
	});
});
