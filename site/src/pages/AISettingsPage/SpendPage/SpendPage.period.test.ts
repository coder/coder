import { afterAll, beforeAll, expect, it, vi } from "vitest";
import { appliedWindowToDateRange, firstDayWithinRetention } from "./SpendPage";

// Local days run ahead of the UTC calendar the server uses in a
// positive-offset zone, which is where the retention mapping has its edges.
beforeAll(() => {
	vi.stubEnv("TZ", "Asia/Tokyo");
});

afterAll(() => {
	vi.unstubAllEnvs();
});

// Noon on March 12 in Asia/Tokyo.
const now = new Date("2026-03-12T03:00:00Z");

const clampedToRetention = (cutoff: Date, period_end: string) => ({
	period_start: cutoff.toISOString(),
	period_end,
	retention_start: cutoff.toISOString(),
});

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
	expect(
		appliedWindowToDateRange(
			clampedToRetention(
				new Date("2026-02-14T15:32:10Z"),
				"2026-03-01T00:00:00Z",
			),
			now,
		),
	).toEqual({
		startDate: new Date(2026, 1, 16),
		endDate: new Date(2026, 2, 1),
	});
});

it("treats a retention cutoff at UTC midnight as a cutoff", () => {
	expect(
		appliedWindowToDateRange(
			clampedToRetention(
				new Date("2026-02-15T00:00:00Z"),
				"2026-03-01T00:00:00Z",
			),
			now,
		),
	).toMatchObject({ startDate: new Date(2026, 1, 16) });
});

it("keeps a budget start whose local midnight precedes the cutoff inside retention", () => {
	expect(
		appliedWindowToDateRange(
			{
				period_start: "2026-02-01T00:00:00Z",
				period_end: "2026-03-01T00:00:00Z",
				retention_start: "2026-01-31T23:30:00Z",
			},
			now,
		),
	).toMatchObject({ startDate: new Date(2026, 1, 2) });
});

it("shows the first selectable day when no whole day of the window remains", () => {
	// 08:30 on April 1 in Asia/Tokyo, still March 31 in UTC, with the cutoff
	// at 23:00 the evening before.
	const lateNow = new Date("2026-03-31T23:30:00Z");
	expect(
		appliedWindowToDateRange(
			clampedToRetention(
				new Date("2026-03-31T14:00:00Z"),
				"2026-04-01T00:00:00Z",
			),
			lateNow,
		),
	).toEqual({
		startDate: new Date(2026, 3, 1),
		endDate: new Date(2026, 3, 1, 9),
	});
});

it("offers no range when retention is shorter than a day", () => {
	const cutoff = new Date(now.getTime() - 60 * 60 * 1000);
	expect(
		appliedWindowToDateRange(
			clampedToRetention(cutoff, "2026-04-01T00:00:00Z"),
			now,
		),
	).toBeUndefined();
});

it("offers no range while the period's first local day is still tomorrow", () => {
	// 19:00 on September 30 in America/Los_Angeles, already October 1 in UTC,
	// so the server has moved on to the October budget period.
	vi.stubEnv("TZ", "America/Los_Angeles");
	expect(
		appliedWindowToDateRange(
			{
				period_start: "2026-10-01T00:00:00Z",
				period_end: "2026-11-01T00:00:00Z",
			},
			new Date("2026-10-01T02:00:00Z"),
		),
	).toBeUndefined();
	vi.stubEnv("TZ", "Asia/Tokyo");
});

it("bounds the picker at the first local midnight after the retention cutoff", () => {
	const cutoff = new Date(2026, 1, 14, 15, 32, 10);
	expect(firstDayWithinRetention(cutoff)).toEqual(new Date(2026, 1, 15));
	expect(firstDayWithinRetention(new Date(2026, 1, 14))).toEqual(
		new Date(2026, 1, 14),
	);
});
