import { describe, expect, it } from "vitest";
import {
	ALL_TIME_PRESET_ID,
	parseLastSeenRange,
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

	it("starts at the Unix epoch when only an end is set", () => {
		expect(parseLastSeenRange({ last_seen_before: end.toISOString() })).toEqual(
			{
				start: new Date(0),
				end,
			},
		);
	});

	it.each([
		["no bounds", {}],
		["only a start", { last_seen_after: start.toISOString() }],
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

	it("sends only the end for a range starting at the Unix epoch", () => {
		expect(
			withLastSeen("status:active", {
				start: new Date(0),
				end,
				preset: "over_30d",
			}),
		).toBe(`status:active last_seen_before:"${end.toISOString()}"`);
	});

	it("clears the range for All time", () => {
		const filtered = withLastSeen("status:active alice", { start, end });
		expect(
			withLastSeen(filtered, { start, end, preset: ALL_TIME_PRESET_ID }),
		).toBe("status:active alice");
	});
});
