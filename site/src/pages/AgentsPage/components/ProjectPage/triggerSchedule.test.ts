import { describe, expect, it } from "vitest";
import {
	describeSchedule,
	draftFromSchedule,
	type ScheduleDraft,
	scheduleFromDraft,
	validateScheduleDraft,
	withSchedulePreset,
} from "./triggerSchedule";

const now = new Date("2026-09-04T12:00:00Z");

const draft = (fields: Partial<ScheduleDraft>): ScheduleDraft => ({
	...draftFromSchedule(undefined, "Europe/Berlin"),
	...fields,
});

describe("schedule drafts", () => {
	it.each([
		[
			draft({ preset: "hourly", minute: "15" }),
			"CRON_TZ=Europe/Berlin 15 * * * *",
		],
		[
			draft({ preset: "daily", time: "07:05" }),
			"CRON_TZ=Europe/Berlin 5 7 * * *",
		],
		[
			draft({ preset: "weekdays", time: "09:00" }),
			"CRON_TZ=Europe/Berlin 0 9 * * 1-5",
		],
		[
			draft({ preset: "weekly", time: "18:30", weekday: "5" }),
			"CRON_TZ=Europe/Berlin 30 18 * * 5",
		],
		[
			draft({ preset: "custom", custom: " 0  9 1 * * " }),
			"CRON_TZ=Europe/Berlin 0 9 1 * *",
		],
	])("saves %o as %s and reads it back", (input, schedule) => {
		expect(scheduleFromDraft(input)).toBe(schedule);
		const read = draftFromSchedule(schedule, "UTC");
		expect(read.preset).toBe(input.preset);
		expect(scheduleFromDraft(read)).toBe(schedule);
	});

	it("reads a schedule without a time zone as UTC, and 7 as Sunday", () => {
		expect(draftFromSchedule("0 9 * * 7", "Europe/Berlin")).toMatchObject({
			preset: "weekly",
			weekday: "0",
			time: "09:00",
			timeZone: "UTC",
		});
	});

	it("opens schedules the presets cannot express as custom", () => {
		expect(
			draftFromSchedule("CRON_TZ=UTC */15 9-17 * * 1-5", "UTC"),
		).toMatchObject({ preset: "custom", custom: "*/15 9-17 * * 1-5" });
	});

	it("starts a custom expression from the schedule picked so far", () => {
		const weekly = draft({ preset: "weekly", time: "08:15", weekday: "2" });
		expect(withSchedulePreset(weekly, "custom").custom).toBe("15 8 * * 2");
	});
});

describe("describeSchedule", () => {
	it.each([
		["CRON_TZ=UTC 0 * * * *", "Every hour"],
		["CRON_TZ=UTC 20 * * * *", "Every hour at 20 minutes past the hour"],
		["CRON_TZ=UTC 0 0 * * *", "Every day at 12:00 AM"],
		["CRON_TZ=UTC 0 9 * * 1-5", "Every weekday at 9:00 AM"],
		["CRON_TZ=UTC 30 14 * * 1", "Every Monday at 2:30 PM"],
		["CRON_TZ=UTC 0 9 1 * *", "At 9:00 AM, on day 1 of the month"],
		["CRON_TZ=UTC 0 10 * * 1-3", "At 10:00 AM, Monday through Wednesday"],
	])("describes %s as %s", (schedule, description) => {
		expect(describeSchedule(schedule)).toBe(description);
	});
});

describe("validateScheduleDraft", () => {
	it("accepts the presets and a valid custom expression", () => {
		expect(validateScheduleDraft(draft({}), now)).toEqual({});
		expect(
			validateScheduleDraft(
				draft({ preset: "custom", custom: "*/5 * * * *" }),
				now,
			),
		).toEqual({});
	});

	it.each([
		[draft({ preset: "hourly", minute: "60" }), "minute"],
		[draft({ preset: "daily", time: "" }), "time"],
		[draft({ preset: "custom", custom: "0 9 * *" }), "custom"],
		[draft({ preset: "custom", custom: "CRON_TZ=UTC 0 9 * * *" }), "custom"],
		[draft({ preset: "custom", custom: "0 25 * * *" }), "custom"],
		[draft({ preset: "custom", custom: "*/2 * * * *" }), "custom"],
	])("flags %o in its %s field", (input, field) => {
		expect(Object.keys(validateScheduleDraft(input, now))).toEqual([field]);
	});
});
