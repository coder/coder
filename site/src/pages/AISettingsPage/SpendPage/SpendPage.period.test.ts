import { expect, it } from "vitest";
import { appliedWindowToDateRange } from "./SpendPage";

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
	).toEqual({ startDate: new Date(2026, 2, 1), endDate: new Date(2026, 3, 1) });
});

it("starts the day after a mid-day retention cutoff", () => {
	expect(
		appliedWindowToDateRange(
			{
				period_start: "2026-02-14T15:32:10Z",
				period_end: "2026-03-01T00:00:00Z",
			},
			now,
		),
	).toEqual({
		startDate: new Date(2026, 1, 15),
		endDate: new Date(2026, 2, 1),
	});
});
