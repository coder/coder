import { expect, it } from "vitest";
import { clampSpendPeriod } from "./SpendFilters";

it("trims a range that outlasts the export limit by a daylight-saving hour", () => {
	const startDate = new Date("2026-10-10T04:00:00Z");
	const clamped = clampSpendPeriod({
		startDate,
		endDate: new Date("2026-11-10T05:00:00Z"),
	});
	expect(clamped).toEqual({
		startDate,
		endDate: new Date("2026-11-10T04:00:00Z"),
	});
});

it("leaves a range within the export limit unchanged", () => {
	const range = {
		startDate: new Date("2026-03-01T00:00:00Z"),
		endDate: new Date("2026-04-01T00:00:00Z"),
	};
	expect(clampSpendPeriod(range)).toBe(range);
});
