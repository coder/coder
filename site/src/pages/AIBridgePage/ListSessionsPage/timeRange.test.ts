import { describe, expect, it } from "vitest";
import {
	defaultTimeRange,
	fromLocalInputValue,
	parseTimeRange,
	setTimeRangeInQuery,
	type TimeRange,
	toLocalInputValue,
	toRFC3339,
	withDefaultTimeRange,
} from "./timeRange";

describe("toLocalInputValue", () => {
	it("formats a date as local YYYY-MM-DDTHH:mm", () => {
		expect(toLocalInputValue(new Date(2026, 7, 13, 9, 5))).toBe(
			"2026-08-13T09:05",
		);
	});
});

describe("fromLocalInputValue", () => {
	it("parses a datetime-local string as local time", () => {
		expect(fromLocalInputValue("2026-08-13T09:05")).toEqual(
			new Date(2026, 7, 13, 9, 5),
		);
	});

	it("returns null for empty or malformed input", () => {
		expect(fromLocalInputValue("")).toBeNull();
		expect(fromLocalInputValue("not-a-date")).toBeNull();
	});

	it("round-trips with toLocalInputValue", () => {
		const date = new Date(2026, 0, 2, 23, 59);
		expect(fromLocalInputValue(toLocalInputValue(date))).toEqual(date);
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
		const now = new Date(Date.UTC(2026, 7, 13, 15, 0, 0));
		const range = defaultTimeRange(now);
		expect(range.startedBefore).toEqual(now);
		expect(range.startedAfter).toEqual(
			new Date(now.getTime() - 24 * 60 * 60 * 1000),
		);
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
