import { describe, expect, it } from "vitest";
import { formatUsageDateRange, toInclusiveDateRange } from "./dateRange";

describe("toInclusiveDateRange", () => {
	it("subtracts 1ms when endDateIsExclusive is true and end date is midnight", () => {
		const startDate = new Date("2025-06-01T00:00:00.000");
		const endDate = new Date("2025-06-08T00:00:00.000");
		const result = toInclusiveDateRange({ startDate, endDate }, true);
		expect(result.endDate.getTime()).toBe(endDate.getTime() - 1);
	});

	it("returns unchanged when endDateIsExclusive is true and end date is not midnight", () => {
		const startDate = new Date("2025-06-01T00:00:00.000");
		const endDate = new Date("2025-06-08T14:30:00.000");
		const result = toInclusiveDateRange({ startDate, endDate }, true);
		expect(result.endDate).toBe(endDate);
	});

	it("returns unchanged when endDateIsExclusive is false and end date is midnight", () => {
		const startDate = new Date("2025-06-01T00:00:00.000");
		const endDate = new Date("2025-06-08T00:00:00.000");
		const result = toInclusiveDateRange({ startDate, endDate }, false);
		expect(result.endDate).toBe(endDate);
	});

	it("returns unchanged when endDateIsExclusive is false and end date is not midnight", () => {
		const startDate = new Date("2025-06-01T00:00:00.000");
		const endDate = new Date("2025-06-08T14:30:00.000");
		const result = toInclusiveDateRange({ startDate, endDate }, false);
		expect(result.endDate).toBe(endDate);
	});

	it("preserves startDate in all cases", () => {
		const startDate = new Date("2025-06-01T00:00:00.000");
		const midnightEnd = new Date("2025-06-08T00:00:00.000");
		const nonMidnightEnd = new Date("2025-06-08T14:30:00.000");

		const explicitMidnight = toInclusiveDateRange(
			{ startDate, endDate: midnightEnd },
			true,
		);
		expect(explicitMidnight.startDate).toBe(startDate);

		const explicitNonMidnight = toInclusiveDateRange(
			{ startDate, endDate: nonMidnightEnd },
			true,
		);
		expect(explicitNonMidnight.startDate).toBe(startDate);

		const implicitMidnight = toInclusiveDateRange(
			{ startDate, endDate: midnightEnd },
			false,
		);
		expect(implicitMidnight.startDate).toBe(startDate);

		const implicitNonMidnight = toInclusiveDateRange(
			{ startDate, endDate: nonMidnightEnd },
			false,
		);
		expect(implicitNonMidnight.startDate).toBe(startDate);
	});
});

describe("formatUsageDateRange", () => {
	it("formats a basic date range without options", () => {
		const result = formatUsageDateRange({
			startDate: new Date("2025-06-01T00:00:00.000"),
			endDate: new Date("2025-06-08T00:00:00.000"),
		});
		expect(result).toBe("Jun 1 – Jun 8, 2025");
	});

	it("shows previous day when endDateIsExclusive is true and end date is midnight", () => {
		const result = formatUsageDateRange(
			{
				startDate: new Date("2025-06-01T00:00:00.000"),
				endDate: new Date("2025-06-08T00:00:00.000"),
			},
			{ endDateIsExclusive: true },
		);
		expect(result).toBe("Jun 1 – Jun 7, 2025");
	});

	it("shows same day when endDateIsExclusive is true and end date is not midnight", () => {
		const result = formatUsageDateRange(
			{
				startDate: new Date("2025-06-01T00:00:00.000"),
				endDate: new Date("2025-06-08T14:30:00.000"),
			},
			{ endDateIsExclusive: true },
		);
		expect(result).toBe("Jun 1 – Jun 8, 2025");
	});

	it("shows same day when endDateIsExclusive is false and end date is midnight", () => {
		const result = formatUsageDateRange(
			{
				startDate: new Date("2025-06-01T00:00:00.000"),
				endDate: new Date("2025-06-08T00:00:00.000"),
			},
			{ endDateIsExclusive: false },
		);
		expect(result).toBe("Jun 1 – Jun 8, 2025");
	});

	it("formats a cross-month range", () => {
		const result = formatUsageDateRange({
			startDate: new Date("2025-05-28T00:00:00.000"),
			endDate: new Date("2025-06-04T00:00:00.000"),
		});
		expect(result).toBe("May 28 – Jun 4, 2025");
	});

	it("formats a same-month range", () => {
		const result = formatUsageDateRange({
			startDate: new Date("2025-06-01T00:00:00.000"),
			endDate: new Date("2025-06-15T00:00:00.000"),
		});
		expect(result).toBe("Jun 1 – Jun 15, 2025");
	});

	it("formats a cross-year range without ambiguity", () => {
		const result = formatUsageDateRange({
			startDate: new Date("2025-12-28T00:00:00.000"),
			endDate: new Date("2026-01-04T00:00:00.000"),
		});
		// Start date omits year, end date includes it. The label reads
		// as "Dec 28 – Jan 4, 2026" which is unambiguous enough for a
		// 30-day range label (the start year is implied).
		expect(result).toBe("Dec 28 – Jan 4, 2026");
	});
});
