import { describe, expect, it } from "vitest";
import { matchRepeatShortcut } from "./AutomationScheduleFields";

describe("matchRepeatShortcut", () => {
	it.each([
		{ cron: "*/5 * * * *", expected: { repeat: "every-5-minutes" } },
		{ cron: "*/15 * * * *", expected: { repeat: "every-15-minutes" } },
		{ cron: "15 * * * *", expected: { repeat: "hourly", minute: 15 } },
		{ cron: "0 9 * * *", expected: { repeat: "daily", hour: 9, minute: 0 } },
		{
			cron: " 30 9 * * 1-5 ",
			expected: { repeat: "weekdays", hour: 9, minute: 30 },
		},
		{
			cron: "59 23 * * 1",
			expected: { repeat: "weekly-monday", hour: 23, minute: 59 },
		},
	])("matches $cron", ({ cron, expected }) => {
		expect(matchRepeatShortcut(cron)).toEqual(expected);
	});

	// Equivalent schedules in other forms stay Custom so the user sees the
	// stored string instead of a rewrite.
	it.each([
		"30 09 * * 1-5",
		"30  9 * * 1-5",
		"60 9 * * *",
		"0 24 * * *",
		"0 9 * * 1,2,3,4,5",
		"0 9 1 * *",
		"*/10 * * * *",
		"",
	])("treats %j as Custom", (cron) => {
		expect(matchRepeatShortcut(cron)).toBeUndefined();
	});
});
