import { describe, expect, it } from "vitest";
import {
	ALL_TIME_PRESET_ID,
	EPOCH,
	parseLastSeenRange,
	resolveLastSeen,
	resolveRelativeLastSeen,
	withLastSeen,
} from "./lastSeenRange";

const start = new Date("2026-03-05T12:00:00.000Z");
const end = new Date("2026-03-12T12:00:00.000Z");
const epochFilter = 'last_seen_after:"1970-01-01T00:00:00.000Z"';

describe("parseLastSeenRange", () => {
	it("returns the range when both bounds are valid and ordered", () => {
		expect(
			parseLastSeenRange(
				{
					last_seen_after: start.toISOString(),
					last_seen_before: end.toISOString(),
				},
				end,
			),
		).toEqual({ start, end });
	});

	it("resolves relative bounds against the given time", () => {
		expect(
			parseLastSeenRange(
				{ last_seen_after: "-7d", last_seen_before: "-24h" },
				end,
			),
		).toEqual({ start, end: new Date("2026-03-11T12:00:00.000Z") });
	});

	it.each([
		["no bounds", {}],
		["only a start", { last_seen_after: start.toISOString() }],
		["only an end", { last_seen_before: end.toISOString() }],
		["an invalid date", { last_seen_after: "nope", last_seen_before: "x" }],
		[
			"a reversed range",
			{
				last_seen_after: end.toISOString(),
				last_seen_before: start.toISOString(),
			},
		],
	])("returns undefined for a user with %s", (_, values) => {
		expect(parseLastSeenRange(values, end)).toBeUndefined();
	});
});

describe("withLastSeen", () => {
	it("stores a preset as relative values while keeping chips and free-text search", () => {
		expect(
			withLastSeen("status:active alice", { start, end, preset: "last_7d" }),
		).toBe("status:active alice last_seen_after:-7d");
	});

	it("stores Over presets with the epoch start so never seen users are excluded", () => {
		expect(
			withLastSeen("status:active", { start: EPOCH, end, preset: "over_30d" }),
		).toBe(`status:active ${epochFilter} last_seen_before:-30d`);
	});

	it("stores a custom range as timestamps, replacing an existing range", () => {
		expect(withLastSeen("role:owner last_seen_after:-7d", { start, end })).toBe(
			`role:owner last_seen_after:"${start.toISOString()}" last_seen_before:"${end.toISOString()}"`,
		);
	});

	it("clears the range for All time", () => {
		expect(
			withLastSeen("status:active alice last_seen_after:-7d", {
				start,
				end,
				preset: ALL_TIME_PRESET_ID,
			}),
		).toBe("status:active alice");
	});
});

describe("resolveLastSeen", () => {
	it("recognizes a preset from its relative values", () => {
		expect(
			resolveLastSeen(
				{
					last_seen_after: EPOCH.toISOString(),
					last_seen_before: "-30d",
				},
				end,
			),
		).toEqual({
			start: EPOCH,
			end: new Date("2026-02-10T12:00:00.000Z"),
			preset: "over_30d",
		});
	});

	it("uses the filter range when it matches no preset", () => {
		expect(
			resolveLastSeen(
				{
					last_seen_after: start.toISOString(),
					last_seen_before: end.toISOString(),
				},
				end,
			),
		).toEqual({ start, end });
	});

	it("falls back to All time", () => {
		expect(resolveLastSeen({}, end)).toEqual({
			start: EPOCH,
			end,
			preset: ALL_TIME_PRESET_ID,
		});
	});
});

describe("resolveRelativeLastSeen", () => {
	it("rewrites relative values as timestamps and keeps the rest of the query", () => {
		expect(
			resolveRelativeLastSeen(
				`status:active alice ${epochFilter} last_seen_before:-30d`,
				end,
			),
		).toBe(
			`status:active alice ${epochFilter} last_seen_before:"2026-02-10T12:00:00.000Z"`,
		);
	});

	it("resolves hour values", () => {
		expect(resolveRelativeLastSeen("last_seen_after:-24h", end)).toBe(
			'last_seen_after:"2026-03-11T12:00:00.000Z"',
		);
	});

	it("leaves a query without relative values unchanged", () => {
		const query = `status:active last_seen_after:"${start.toISOString()}"`;
		expect(resolveRelativeLastSeen(query, end)).toBe(query);
	});
});
