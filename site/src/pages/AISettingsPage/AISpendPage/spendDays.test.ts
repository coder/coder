import dayjs from "dayjs";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	defaultSpendDays,
	fromPickerValue,
	parseSpendDays,
	toPickerNow,
	toPickerValue,
	toSpendPeriod,
} from "./spendDays";

describe("defaultSpendDays", () => {
	it("covers the last 7 days, including today", () => {
		expect(defaultSpendDays(new Date("2026-03-12T15:30:00Z"))).toEqual({
			startDate: "2026-03-06",
			endDate: "2026-03-12",
		});
	});
});

describe("parseSpendDays", () => {
	it.each([
		["a single day", "2026-03-12", "2026-03-12"],
		["the longest range", "2026-01-01", "2026-01-31"],
	])("accepts %s", (_, startDate, endDate) => {
		expect(parseSpendDays(startDate, endDate)).toEqual({ startDate, endDate });
	});

	it.each([
		["a missing start", null, "2026-03-12"],
		["a missing end", "2026-03-12", null],
		["a timestamp", "2026-03-06T00:00:00Z", "2026-03-12"],
		["an impossible date", "2026-02-30", "2026-03-12"],
		["a reversed range", "2026-03-12", "2026-03-06"],
		["a range over 31 days", "2026-01-01", "2026-02-01"],
	])("rejects %s", (_, startDate, endDate) => {
		expect(parseSpendDays(startDate, endDate)).toBeNull();
	});
});

describe("toSpendPeriod", () => {
	it("bounds the days by UTC midnights with an exclusive end", () => {
		expect(
			toSpendPeriod({ startDate: "2026-03-06", endDate: "2026-03-12" }),
		).toEqual({
			period_start: "2026-03-06T00:00:00.000Z",
			period_end: "2026-03-13T00:00:00.000Z",
		});
	});
});

describe("fromPickerValue", () => {
	it("reads the last day from a range ending at midnight", () => {
		expect(
			fromPickerValue({
				startDate: new Date(2026, 2, 6),
				endDate: new Date(2026, 2, 13),
			}),
		).toEqual({ startDate: "2026-03-06", endDate: "2026-03-12" });
	});

	it("reads today from a range ending at the next hour", () => {
		expect(
			fromPickerValue({
				startDate: new Date(2026, 2, 6),
				endDate: new Date(2026, 2, 12, 16),
			}),
		).toEqual({ startDate: "2026-03-06", endDate: "2026-03-12" });
	});

	it("round-trips the days through the picker", () => {
		const days = { startDate: "2026-02-27", endDate: "2026-03-12" };
		expect(fromPickerValue(toPickerValue(days))).toEqual(days);
	});
});

describe("outside UTC", () => {
	afterEach(() => {
		vi.unstubAllEnvs();
	});

	it.each([
		{
			name: "behind UTC, where the UTC day has already changed",
			tz: "America/Los_Angeles",
			nowUTC: "2026-11-20T04:00:00Z",
			days: { startDate: "2026-11-14", endDate: "2026-11-20" },
			period: {
				period_start: "2026-11-14T00:00:00.000Z",
				period_end: "2026-11-21T00:00:00.000Z",
			},
		},
		{
			name: "ahead of UTC, where the UTC day has not changed yet",
			tz: "Asia/Tokyo",
			nowUTC: "2026-11-19T16:00:00Z",
			days: { startDate: "2026-11-13", endDate: "2026-11-19" },
			period: {
				period_start: "2026-11-13T00:00:00.000Z",
				period_end: "2026-11-20T00:00:00.000Z",
			},
		},
	])("defaults to the UTC days $name", ({ tz, nowUTC, days, period }) => {
		vi.stubEnv("TZ", tz);
		const current = new Date(nowUTC);
		expect(defaultSpendDays(current)).toEqual(days);
		expect(toSpendPeriod(defaultSpendDays(current))).toEqual(period);
		expect(dayjs(toPickerNow(current)).format("YYYY-MM-DD")).toBe(days.endDate);
	});

	it("round-trips the days through the picker across a daylight-saving change", () => {
		vi.stubEnv("TZ", "America/Los_Angeles");
		const days = { startDate: "2026-10-29", endDate: "2026-11-04" };
		expect(fromPickerValue(toPickerValue(days))).toEqual(days);
	});
});
