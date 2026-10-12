import { describe, expect, it } from "vitest";
import {
	cronFixedTime,
	describeCronProblem,
	describeSchedule,
	matchRepeatShortcut,
} from "./AutomationScheduleFields";

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

describe("describeSchedule", () => {
	it.each([
		{ cron: "*/5 * * * *", expected: "Every 5 minutes" },
		{ cron: "*/10 * * * *", expected: "Every 10 minutes" },
		{ cron: " */45 * * * * ", expected: "Every 45 minutes" },
		{ cron: "*/1 * * * *", expected: "Every minute" },
		{ cron: "* * * * *", expected: "Every minute" },
		{ cron: "* * * * 1", expected: "* * * * 1" },
		{ cron: "30 9 * * 1-5", expected: "Weekdays at 09:30" },
		{ cron: "*/60 * * * *", expected: "*/60 * * * *" },
		{ cron: "*/10 9 * * *", expected: "*/10 9 * * *" },
	])("describes $cron as $expected", ({ cron, expected }) => {
		expect(describeSchedule(cron)).toBe(expected);
	});
});

describe("describeCronProblem", () => {
	it.each([
		{ cron: "61 * * * *", expected: "Minute must be 0-59." },
		{ cron: "0 24 * * *", expected: "Hour must be 0-23." },
		{ cron: "0 9 0 * *", expected: "Day of month must be 1-31." },
		{ cron: "0 9 * 1-13 *", expected: "Month must be 1-12." },
		{ cron: "0 9 * * 1-8", expected: "Day of week must be 0-6." },
		{
			cron: "@daily",
			expected:
				"Use five fields: minute, hour, day of month, month, and day of week, such as 0 9 * * 1-5. Shortcuts such as @daily are not supported.",
		},
		{
			cron: "0 9 * *",
			expected:
				"Use five fields: minute, hour, day of month, month, and day of week, such as 0 9 * * 1-5.",
		},
		{ cron: "*/90 * * * *", expected: undefined },
		{ cron: "0,30 9-17 1 1 MON-FRI", expected: undefined },
		{ cron: "", expected: undefined },
	])("describes $cron", ({ cron, expected }) => {
		expect(describeCronProblem(cron)).toBe(expected);
	});
});

describe("cronFixedTime", () => {
	it.each([
		{ cron: "0 10 15 * *", expected: "10:00" },
		{ cron: "5 7 * * 1-5", expected: "07:05" },
		{ cron: "*/5 * * * *", expected: undefined },
		{ cron: "0 9,17 * * *", expected: undefined },
		{ cron: "0 9 * *", expected: undefined },
		{ cron: "61 9 * * *", expected: undefined },
	])("reads $cron as $expected", ({ cron, expected }) => {
		expect(cronFixedTime(cron)).toBe(expected);
	});
});
