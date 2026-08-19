import { describe, expect, it } from "vitest";
import type { FullTimeRange } from "#/components/DateTimeRangeFilter/timeRange";
import {
	defaultTimeRange,
	parseTimeRange,
	queryWithTimeRange,
	toRFC3339,
	withDefaultTimeRange,
} from "./timeRange";

const now = new Date(2026, 7, 13, 15, 0, 0);

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

describe("withDefaultTimeRange", () => {
	const range: FullTimeRange = {
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

	it("does not mistake a quoted value containing the key for a bound", () => {
		// A quoted value that mentions started_after is not a time bound.
		const query = 'session_id:"started_after:2026-08-01"';
		expect(withDefaultTimeRange(query, range)).toBe(
			'session_id:"started_after:2026-08-01" started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"',
		);
	});
});

describe("queryWithTimeRange", () => {
	const range: FullTimeRange = {
		startedAfter: new Date(Date.UTC(2026, 7, 12, 15, 0, 0)),
		startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
	};

	it("preserves other filters and replaces the time range", () => {
		expect(
			queryWithTimeRange(
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

	it("returns null when neither bound is set", () => {
		expect(parseTimeRange({})).toBeNull();
	});

	it("returns a partial range when only one bound is set", () => {
		expect(parseTimeRange({ started_after: "2026-08-12T15:00:00Z" })).toEqual({
			startedAfter: new Date(Date.UTC(2026, 7, 12, 15, 0, 0)),
			startedBefore: undefined,
		});
		expect(parseTimeRange({ started_before: "2026-08-13T15:00:00Z" })).toEqual({
			startedAfter: undefined,
			startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
		});
	});

	it("drops malformed bounds", () => {
		expect(
			parseTimeRange({
				started_after: "bogus",
				started_before: "2026-08-13T15:00:00Z",
			}),
		).toEqual({
			startedAfter: undefined,
			startedBefore: new Date(Date.UTC(2026, 7, 13, 15, 0, 0)),
		});
		expect(parseTimeRange({ started_after: "bogus" })).toBeNull();
	});
});
