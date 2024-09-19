import {
	consolidateTimings,
	intervals,
	startOffset,
	totalDuration,
} from "./timings";

test("totalDuration", () => {
	const timings = [
		{ started_at: "2021-01-01T00:00:10Z", ended_at: "2021-01-01T00:00:20Z" },
		{ started_at: "2021-01-01T00:00:18Z", ended_at: "2021-01-01T00:00:34Z" },
		{ started_at: "2021-01-01T00:00:20Z", ended_at: "2021-01-01T00:00:30Z" },
	];
	expect(totalDuration(timings)).toBe(24);
});

test("intervals", () => {
	expect(intervals(24, 5)).toEqual([5, 10, 15, 20, 25]);
	expect(intervals(25, 5)).toEqual([5, 10, 15, 20, 25]);
	expect(intervals(26, 5)).toEqual([5, 10, 15, 20, 25, 30]);
});

test("consolidateTimings", () => {
	const timings = [
		{ started_at: "2021-01-01T00:00:10Z", ended_at: "2021-01-01T00:00:22Z" },
		{ started_at: "2021-01-01T00:00:18Z", ended_at: "2021-01-01T00:00:34Z" },
		{ started_at: "2021-01-01T00:00:20Z", ended_at: "2021-01-01T00:00:30Z" },
	];
	const timing = consolidateTimings(timings);
	expect(timing.started_at).toBe("2021-01-01T00:00:10.000Z");
	expect(timing.ended_at).toBe("2021-01-01T00:00:34.000Z");
});

test("startOffset", () => {
	const timings = [
		{ started_at: "2021-01-01T00:00:10Z", ended_at: "2021-01-01T00:00:22Z" },
		{ started_at: "2021-01-01T00:00:18Z", ended_at: "2021-01-01T00:00:34Z" },
		{ started_at: "2021-01-01T00:00:20Z", ended_at: "2021-01-01T00:00:30Z" },
	];
	const consolidated = consolidateTimings(timings);
	const timing = timings[1];
	expect(startOffset(consolidated, timing)).toBe(8);
});
