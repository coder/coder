import { describe, expect, it } from "vitest";
import { formatTriggerLabel, parseTimeExpression } from "./timeRange";

const now = new Date(2026, 7, 13, 15, 0, 0);

describe("parseTimeExpression", () => {
	it("parses now case-insensitively", () => {
		expect(parseTimeExpression("now", now)).toEqual(now);
		expect(parseTimeExpression("Now", now)).toEqual(now);
		expect(parseTimeExpression(" NOW ", now)).toEqual(now);
	});

	it("parses clock times against the current day", () => {
		expect(parseTimeExpression("15:43", now)).toEqual(
			new Date(2026, 7, 13, 15, 43, 0),
		);
		expect(parseTimeExpression("09:05:09", now)).toEqual(
			new Date(2026, 7, 13, 9, 5, 9),
		);
	});

	it("defaults a bare date to midnight", () => {
		expect(parseTimeExpression("2026-08-13", now)).toEqual(
			new Date(2026, 7, 13),
		);
	});

	it("parses date and time together", () => {
		expect(parseTimeExpression("2026-08-13 11:43", now)).toEqual(
			new Date(2026, 7, 13, 11, 43, 0),
		);
		expect(parseTimeExpression("2026-08-13 11:43:21", now)).toEqual(
			new Date(2026, 7, 13, 11, 43, 21),
		);
	});

	it("rejects single-digit hours", () => {
		expect(parseTimeExpression("9:05", now)).toBeNull();
		expect(parseTimeExpression("2026-08-13 7:23:00", now)).toBeNull();
	});

	it("rejects out-of-range clocks and dates", () => {
		expect(parseTimeExpression("23:59:99", now)).toBeNull();
		expect(parseTimeExpression("24:00", now)).toBeNull();
		expect(parseTimeExpression("2026-02-30", now)).toBeNull();
		expect(parseTimeExpression("2026-13-01", now)).toBeNull();
		expect(parseTimeExpression("2026-08-13 23:59:99", now)).toBeNull();
	});

	it("rejects unknown shapes", () => {
		expect(parseTimeExpression("", now)).toBeNull();
		expect(parseTimeExpression("30d", now)).toBeNull();
		expect(parseTimeExpression("13/08/2026", now)).toBeNull();
		expect(parseTimeExpression("2026-08-13T11:43", now)).toBeNull();
	});
});

describe("formatTriggerLabel", () => {
	it("collapses a single day", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 3, 10, 7, 23, 0),
					startedBefore: new Date(2026, 3, 10, 9, 30, 0),
				},
				now,
			),
		).toBe("Apr 10");
	});

	it("labels a range ending today against now", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 7, 11, 23, 59, 59),
					startedBefore: new Date(2026, 7, 13, 10, 0, 0),
				},
				now,
			),
		).toBe("Aug 11 - Today");
	});

	it("shortens ranges within one month", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 3, 17),
					startedBefore: new Date(2026, 3, 19),
				},
				now,
			),
		).toBe("Apr 17 - 19");
	});

	it("falls back to a full range across months", () => {
		expect(
			formatTriggerLabel(
				{
					startedAfter: new Date(2026, 2, 30),
					startedBefore: new Date(2026, 3, 2),
				},
				now,
			),
		).toBe("Mar 30 - Apr 2");
	});
});
