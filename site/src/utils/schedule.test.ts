import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import type {
	Template,
	Workspace,
	WorkspaceBuildDeadlineChange,
	WorkspaceBuildDeadlineSource,
} from "#/api/typesGenerated";
import * as Mocks from "#/testHelpers/entities";
import {
	deadlineChangeMessage,
	deadlineExtensionMax,
	deadlineExtensionMin,
	extractTimezone,
	getMaxDeadline,
	getMaxDeadlineChange,
	getMinDeadline,
	quietHoursDisplay,
	stripTimezone,
} from "./schedule";

dayjs.extend(duration);
const now = dayjs();
const startTime = dayjs(Mocks.MockWorkspaceBuild.updated_at);

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
		const workspace: Workspace = {
			...Mocks.MockWorkspace,
			latest_build: {
				...Mocks.MockWorkspaceBuild,
				deadline: startTime.add(8, "hours").utc().format(),
			},
		};
		it("should be 24 hours from the workspace start time", () => {
			const delta = getMaxDeadline(workspace).diff(startTime);
			expect(delta).toEqual(deadlineExtensionMax.asMilliseconds());
		});
	});

	describe("minDeadline", () => {
		it("should never be less than 30 minutes", () => {
			const delta = getMinDeadline().diff(now);
			expect(delta).toBeGreaterThanOrEqual(
				deadlineExtensionMin.asMilliseconds(),
			);
		});
	});

	describe("getMaxDeadlineChange", () => {
		it("should return the number of hours you can add before hitting the max deadline", () => {
			const deadline = dayjs();
			const maxDeadline = dayjs().add(1, "hour").add(40, "minutes");
			// you can only add one hour even though the max is 1:40 away
			expect(getMaxDeadlineChange(deadline, maxDeadline)).toEqual(1);
		});

		it("should return the number of hours you can subtract before hitting the min deadline", () => {
			const deadline = dayjs().add(2, "hours").add(40, "minutes");
			const minDeadline = dayjs();
			// you can only subtract 2 hours even though the min is 2:40 less
			expect(getMaxDeadlineChange(deadline, minDeadline)).toEqual(2);
		});
	});

	describe("deadlineChangeMessage", () => {
		beforeEach(() => {
			vi.useFakeTimers({ now: new Date("2025-07-15T14:30:00.000Z") });
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		const mockUnattributedChange = {
			changed_at: "2025-07-15T14:28:00.000Z",
			changed_by: null,
			sources: [],
		};
		const mockAttributedChange = {
			...mockUnattributedChange,
			changed_by: {
				id: Mocks.MockUserOwner.id,
				username: Mocks.MockUserOwner.username,
			},
		};
		const mockSources: WorkspaceBuildDeadlineSource[] = [
			{ type: "session_app", app_name: "vscodium", display_name: "VSCodium" },
			{ type: "session_app", app_name: "codium", display_name: "VSCodium" },
			{ type: "session_app", app_name: "cursor", display_name: "Cursor" },
			{ type: "session_app", app_name: "ssh", display_name: "SSH" },
		];

		it.each<[WorkspaceBuildDeadlineChange, string | undefined, Template?]>([
			[
				{
					...mockUnattributedChange,
					reason: "activity",
					sources: mockSources.slice(0, 3),
				},
				"Activity from VSCodium, VSCodium, and Cursor extended this deadline 2 minutes ago.",
			],
			[
				{ ...mockUnattributedChange, reason: "activity", sources: mockSources },
				"Activity from VSCodium, VSCodium, Cursor, and 1 more extended this deadline 2 minutes ago.",
			],
			[
				{ ...mockAttributedChange, reason: "user_changed" },
				"TestUser changed this deadline 2 minutes ago.",
			],
			[
				{ ...mockUnattributedChange, reason: "user_changed" },
				"A user changed this deadline 2 minutes ago.",
			],
			[
				{ ...mockAttributedChange, reason: "autostop_disabled" },
				"TestUser turned off autostop 2 minutes ago.",
			],
			[
				{ ...mockUnattributedChange, reason: "autostop_disabled" },
				"A user turned off autostop 2 minutes ago.",
			],
			[
				{ ...mockAttributedChange, reason: "template_schedule" },
				"A schedule change by TestUser on the Test Template template set this deadline 2 minutes ago.",
			],
			[
				{ ...mockUnattributedChange, reason: "template_schedule" },
				"A schedule change on the test-template template set this deadline 2 minutes ago.",
				{ ...Mocks.MockTemplate, display_name: "" },
			],
			[{ ...mockAttributedChange, reason: "build" }, undefined],
			[
				{ ...mockUnattributedChange, reason: "activity" },
				"Activity extended this deadline 2 minutes ago.",
			],
		])("explains %j", (change, expected, template = Mocks.MockTemplate) => {
			expect(deadlineChangeMessage(change, template)).toBe(expected);
		});
	});

	describe("quietHoursDisplay", () => {
		it("midnight in Poland", () => {
			const quietHoursStart = quietHoursDisplay(
				"pl",
				"00:00",
				"Australia/Sydney",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			expect(quietHoursStart).toBe(
				"00:00 tomorrow (in 9 hours) in Australia/Sydney",
			);
		});
		it("five o'clock today in Sweden", () => {
			const quietHoursStart = quietHoursDisplay(
				"sv",
				"17:00",
				"Europe/London",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			expect(quietHoursStart).toBe(
				"17:00 today (in 11 hours) in Europe/London",
			);
		});
		it("five o'clock today in Finland", () => {
			const quietHoursStart = quietHoursDisplay(
				"fl",
				"17:00",
				"Europe/London",
				new Date("2023-09-06T15:00:00.000+10:00"),
			);

			expect(quietHoursStart).toBe(
				"5:00 PM today (in 11 hours) in Europe/London",
			);
		});
		it("lunch tomorrow in England", () => {
			const quietHoursStart = quietHoursDisplay(
				"en",
				"13:00",
				"US/Central",
				new Date("2023-09-06T08:00:00.000+10:00"),
			);

			expect(quietHoursStart).toBe(
				"1:00 PM tomorrow (in 20 hours) in US/Central",
			);
		});
	});
});
