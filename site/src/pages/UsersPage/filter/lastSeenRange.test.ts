import { describe, expect, it } from "vitest";
import {
	ALL_TIME_PRESET_ID,
	lastSeenUrlState,
	parseLastSeenRange,
	resolveLastSeen,
	withLastSeen,
} from "./lastSeenRange";

const start = new Date("2026-03-05T12:00:00.000Z");
const end = new Date("2026-03-12T12:00:00.000Z");

describe("parseLastSeenRange", () => {
	it("returns the range when both bounds are valid and ordered", () => {
		expect(
			parseLastSeenRange({
				last_seen_after: start.toISOString(),
				last_seen_before: end.toISOString(),
			}),
		).toEqual({ start, end });
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
	])("matches every user with %s", (_, values) => {
		expect(parseLastSeenRange(values)).toBeUndefined();
	});
});

describe("withLastSeen", () => {
	it("adds the range while keeping chips and free-text search", () => {
		expect(withLastSeen("status:active alice", { start, end })).toBe(
			`status:active alice last_seen_after:"${start.toISOString()}" last_seen_before:"${end.toISOString()}"`,
		);
	});

	it("replaces an existing range", () => {
		const previous = withLastSeen("role:owner", {
			start: new Date("2026-01-01T00:00:00.000Z"),
			end,
		});
		expect(withLastSeen(previous, { start, end })).toBe(
			`role:owner last_seen_after:"${start.toISOString()}" last_seen_before:"${end.toISOString()}"`,
		);
	});

	it("sends the epoch start for Over presets so never seen users are excluded", () => {
		expect(
			withLastSeen("status:active", {
				start: new Date(0),
				end,
				preset: "over_30d",
			}),
		).toBe(
			`status:active last_seen_after:"1970-01-01T00:00:00.000Z" last_seen_before:"${end.toISOString()}"`,
		);
	});

	it("clears the range for All time", () => {
		const filtered = withLastSeen("status:active alice", { start, end });
		expect(
			withLastSeen(filtered, { start, end, preset: ALL_TIME_PRESET_ID }),
		).toBe("status:active alice");
	});
});

describe("resolveLastSeen", () => {
	it("resolves a preset against the given time", () => {
		expect(resolveLastSeen("over_30d", {}, end)).toEqual({
			start: new Date(0),
			end: new Date("2026-02-10T12:00:00.000Z"),
			preset: "over_30d",
		});
	});

	it("uses the filter range when there is no known preset", () => {
		expect(
			resolveLastSeen(
				"unknown",
				{
					last_seen_after: start.toISOString(),
					last_seen_before: end.toISOString(),
				},
				end,
			),
		).toEqual({ start, end });
	});

	it("falls back to All time", () => {
		expect(resolveLastSeen(null, {}, end)).toEqual({
			start: new Date(0),
			end,
			preset: ALL_TIME_PRESET_ID,
		});
	});
});

describe("lastSeenUrlState", () => {
	const query = `status:active last_seen_after:"${start.toISOString()}" last_seen_before:"${end.toISOString()}"`;

	it("stores a preset by ID and drops the timestamps", () => {
		expect(lastSeenUrlState(query, { start, end, preset: "last_7d" })).toEqual({
			filter: "status:active",
			preset: "last_7d",
		});
	});

	it("stores a custom range as timestamps", () => {
		expect(lastSeenUrlState("status:active", { start, end })).toEqual({
			filter: query,
			preset: undefined,
		});
	});

	it("clears both for All time", () => {
		expect(
			lastSeenUrlState(query, { start, end, preset: ALL_TIME_PRESET_ID }),
		).toEqual({ filter: "status:active", preset: undefined });
	});
});
