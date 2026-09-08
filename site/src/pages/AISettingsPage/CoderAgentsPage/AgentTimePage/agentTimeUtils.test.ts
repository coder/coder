import { describe, expect, it } from "vitest";
import { MockAgentTimeNow } from "#/testHelpers/agentTime";
import * as utils from "./agentTimeUtils";

describe("agent time UTC calendar controls", () => {
	it("normalizes inclusive picker dates to exclusive calendar boundaries", () => {
		expect(utils.todayUTC(MockAgentTimeNow)).toBe("2026-09-04");
		expect(utils.tomorrowUTC(MockAgentTimeNow)).toBe("2026-09-05");
		const start = utils.dateOnlyToLocalDate("2026-08-29");
		const end = utils.inclusiveLocalDateFromExclusiveEnd("2026-09-05");
		expect(utils.localDateToDateOnly(start)).toBe("2026-08-29");
		expect(
			utils.normalizeDateRange({ startDate: start, endDate: end }),
		).toEqual({ startDate: "2026-08-29", endDate: "2026-09-05" });
		expect(utils.parseDateParam("2026-02-30")).toBeUndefined();
		expect(utils.parseDateParam("2026-09-04")).toBe("2026-09-04");
		expect(utils.formatRange("2026-08-29", "2026-09-05")).toBe(
			"Aug 29, 2026 to Sep 4, 2026",
		);
		expect(utils.formatDate("2026-09-04")).toBe("Sep 4, 2026");
	});
	it("selects supported intervals and preserves explicit URL controls", () => {
		expect(utils.autoInterval(undefined, "2026-09-05")).toBe("month");
		expect(utils.autoInterval("2026-08-29", "2026-09-05")).toBe("day");
		expect(
			utils.approximateBucketCount("day", "2026-08-29", "2026-09-05"),
		).toBe(7);
		expect(utils.intervalLabel("week")).toBe("Weekly");
		for (const option of utils.intervalOptions)
			expect(utils.isAgentTimeInterval(option.value)).toBe(true);
		expect(utils.isAgentTimeInterval("hour")).toBe(false);
		expect(utils.isAgentTimeSortBy("agent_time")).toBe(true);
		expect(utils.isAgentTimeSortBy("runtime")).toBe(false);
		expect(utils.isAgentTimeSortOrder("desc")).toBe(true);
		expect(utils.isAgentTimeTableGroup("user")).toBe(true);
		expect(utils.isAgentTimeTableGroup("group")).toBe(false);
		expect(
			utils.readSearchParam(
				new URLSearchParams("interval=week"),
				"interval",
				utils.isAgentTimeInterval,
				"day",
			),
		).toBe("week");
	});
	it("uses UTC date presets including month and year boundaries", () => {
		const selectedPreset: utils.AgentTimeDatePreset = "today";
		expect(utils.presetRange(selectedPreset, MockAgentTimeNow).startDate).toBe(
			"2026-09-04",
		);
		expect(utils.entityLabel("organization")).toBe("organization");
		expect(utils.presetRange("last_7_days", MockAgentTimeNow)).toEqual({
			startDate: "2026-08-29",
			endDate: "2026-09-05",
		});
		expect(
			utils.activePreset("2026-08-29", "2026-09-05", MockAgentTimeNow),
		).toBe("last_7_days");
		expect(utils.activePreset(undefined, "2026-09-05", MockAgentTimeNow)).toBe(
			"all_history",
		);
		for (const preset of utils.datePresetOptions) {
			if (preset.value !== "all_history")
				expect(
					utils.presetRange(preset.value, MockAgentTimeNow).startDate,
				).toMatch(/^2026-/);
		}
	});
});

describe("precision-safe agent time presentation", () => {
	it("keeps integer milliseconds exact until display", () => {
		expect(utils.parseAgentTimeMs("9007199254740993")).toBe(9007199254740993n);
		expect(utils.formatAgentTimeHours("3600000")).toBe("1.00 hours");
		expect(utils.formatAgentTimeHours(null)).toBe("Unavailable");
		expect(utils.msToHours(null)).toBeNull();
		expect(utils.msToHours("3600000")).toBe(1);
		expect(utils.formatShare("1", "3")).toBe("33.33%");
		expect(utils.formatProcessedMessages("1000")).toBe("1,000");
		expect(utils.shortId("11111111-1111-1111-1111-111111111111")).toBe(
			"11111111...1111",
		);
	});
});
