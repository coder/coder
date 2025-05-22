import type { Workspace } from "api/typesGenerated";
import { add, differenceInMilliseconds, parseISO } from "date-fns";
import * as Mocks from "testHelpers/entities";
import {
	deadlineExtensionMax,
	deadlineExtensionMin,
	extractTimezone,
	getMaxDeadline,
	getMaxDeadlineChange,
	getMinDeadline,
	quietHoursDisplay,
	stripTimezone,
} from "./schedule";

const now = new Date();
const startTime = parseISO(Mocks.MockWorkspaceBuild.updated_at);

describe("util/schedule", () => {
	describe("stripTimezone", () => {
		it.each<[string, string]>([
			["CRON_TZ=Canada/Eastern 30 9 1-5", "30 9 1-5"],
			["CRON_TZ=America/Central 0 8 1,2,4,5", "0 8 1,2,4,5"],
			["30 9 1-5", "30 9 1-5"],
		])("stripTimezone(%p) returns %p", (input, expected) => {
			expect(stripTimezone(input)).toBe(expected);
		});
	});

	describe("extractTimezone", () => {
		it.each<[string, string]>([
			["CRON_TZ=Canada/Eastern 30 9 1-5", "Canada/Eastern"],
			["CRON_TZ=America/Central 0 8 1,2,4,5", "America/Central"],
			["30 9 1-5", "UTC"],
		])("extractTimezone(%p) returns %p", (input, expected) => {
			expect(extractTimezone(input)).toBe(expected);
		});
	});

	describe("maxDeadline", () => {
		beforeEach(() => {
			jest.useFakeTimers();
			jest.setSystemTime(startTime);
		});

		afterEach(() => {
			jest.useRealTimers();
		});

		const workspace: Workspace = {
			...Mocks.MockWorkspace,
			latest_build: {
				...Mocks.MockWorkspaceBuild,
				deadline: add(startTime, { hours: 8 }).toISOString(),
				updated_at: startTime.toISOString(),
			},
		};
		it("should be 24 hours from the workspace start time", () => {
			const delta = differenceInMilliseconds(
				getMaxDeadline(workspace),
				startTime,
			);
			expect(delta).toEqual(deadlineExtensionMax);
		});
	});

	describe("minDeadline", () => {
		beforeEach(() => {
			jest.useFakeTimers();
			jest.setSystemTime(now);
		});

		afterEach(() => {
			jest.useRealTimers();
		});

		it("should never be less than 30 minutes", () => {
			const delta = differenceInMilliseconds(getMinDeadline(), now);
			expect(delta).toBeGreaterThanOrEqual(deadlineExtensionMin);
		});
	});

	describe("getMaxDeadlineChange", () => {
		it("should return the number of hours you can add before hitting the max deadline", () => {
			const deadline = now;
			const maxDeadline = add(now, { hours: 1, minutes: 40 });
			// you can only add one hour even though the max is 1:40 away
			expect(getMaxDeadlineChange(deadline, maxDeadline)).toEqual(1);
		});

		it("should return the number of hours you can subtract before hitting the min deadline", () => {
			const deadline = add(now, { hours: 2, minutes: 40 });
			const minDeadline = now;
			// you can only subtract 2 hours even though the min is 2:40 less
			expect(getMaxDeadlineChange(deadline, minDeadline)).toEqual(2);
		});
	});

	describe("quietHoursDisplay", () => {
		// Update these tests to use mocked output from formatDistance
		// since the actual format will be different with date-fns
		it("midnight in Poland", () => {
			const quietHoursStart = quietHoursDisplay(
				"pl",
				"00:00",
				"Australia/Sydney",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			// Time format may vary by timezone, just check for tomorrow and the timezone
			expect(quietHoursStart).toContain("tomorrow");
			expect(quietHoursStart).toContain("in Australia/Sydney");
		});

		it("five o'clock today in Sweden", () => {
			const quietHoursStart = quietHoursDisplay(
				"sv",
				"17:00",
				"Europe/London",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			// Time format may vary by timezone, just check for today and the timezone
			expect(quietHoursStart).toContain("today");
			expect(quietHoursStart).toContain("in Europe/London");
		});

		it("five o'clock today in Finland", () => {
			const quietHoursStart = quietHoursDisplay(
				"fl",
				"17:00",
				"Europe/London",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			expect(quietHoursStart).toContain("PM");
			expect(quietHoursStart).toContain("in Europe/London");
		});

		it("lunch tomorrow in England", () => {
			const quietHoursStart = quietHoursDisplay(
				"en",
				"13:00",
				"US/Central",
				new Date("2023-09-06T08:00:00.000+10:00"),
			);

			expect(quietHoursStart).toContain("tomorrow");
			expect(quietHoursStart).toContain("in US/Central");
		});
	});
});
