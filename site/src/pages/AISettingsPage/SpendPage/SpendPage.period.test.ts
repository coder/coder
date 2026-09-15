import { expect, it } from "vitest";
import { appliedWindowToDateRange, firstDayWithinRetention } from "./SpendPage";

const now = new Date(2026, 2, 12, 12);

it("maps a UTC budget period onto the same local calendar days", () => {
	expect(
		appliedWindowToDateRange(
			{
				period_start: "2026-03-01T00:00:00Z",
				period_end: "2026-04-01T00:00:00Z",
			},
			now,
		),
	).toEqual({
		startDate: new Date(2026, 2, 1),
		endDate: new Date(2026, 3, 1),
	});
});

it("starts at the first local midnight after a mid-day retention cutoff", () => {
	const cutoff = new Date("2026-02-14T15:32:10Z");
	const { startDate, endDate } = appliedWindowToDateRange(
		{ period_start: cutoff.toISOString(), period_end: "2026-03-01T00:00:00Z" },
		now,
	);
	expect(startDate.getTime()).toBeGreaterThanOrEqual(cutoff.getTime());
	expect(startDate.getTime() - cutoff.getTime()).toBeLessThan(
		24 * 60 * 60 * 1000,
	);
	expect(startDate.getHours()).toBe(0);
	expect(endDate).toEqual(new Date(2026, 2, 1));
});

it("starts today when retention is shorter than a day", () => {
	const cutoff = new Date(now.getTime() - 60 * 60 * 1000);
	const { startDate } = appliedWindowToDateRange(
		{ period_start: cutoff.toISOString(), period_end: "2026-04-01T00:00:00Z" },
		now,
	);
	expect(startDate).toEqual(new Date(2026, 2, 12));
});

it("bounds the picker at the first local midnight after the retention cutoff", () => {
	const cutoff = new Date(2026, 1, 14, 15, 32, 10);
	expect(firstDayWithinRetention(cutoff, now)).toEqual(new Date(2026, 1, 15));
	expect(firstDayWithinRetention(new Date(2026, 1, 14), now)).toEqual(
		new Date(2026, 1, 14),
	);
	expect(
		firstDayWithinRetention(new Date(now.getTime() - 60 * 60 * 1000), now),
	).toEqual(new Date(2026, 2, 12));
});
