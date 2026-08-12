import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import { renderToStaticMarkup } from "react-dom/server";
import type { Workspace } from "#/api/typesGenerated";
import * as Mocks from "#/testHelpers/entities";
import {
	activitySourceLabel,
	autostopDisplay,
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

	describe("activitySourceLabel", () => {
		it.each<[string, string]>([
			["ssh", "SSH"],
			["vscode", "VS Code"],
			["jetbrains", "JetBrains"],
			["reconnecting_pty", "the web terminal"],
			["chat_heartbeat", "AI chat"],
			["app:my-custom-app", "the my-custom-app app"],
			["some_unrecognized_source", "some_unrecognized_source"],
		])("activitySourceLabel(%p) returns %p", (input, expected) => {
			expect(activitySourceLabel(input)).toBe(expected);
		});
	});

	describe("autostopDisplay", () => {
		// MockTemplate already has allow_user_autostop and
		// autostop_requirement set, which is what selects the "Autostop
		// schedule" tooltip branch that the activity line is appended to.
		const template = Mocks.MockTemplate;

		const baseWorkspace: Workspace = {
			...Mocks.MockWorkspace,
			latest_build: {
				...Mocks.MockWorkspaceBuild,
				deadline: dayjs().add(3, "hour").utc().format(),
				status: "running",
			},
		};

		it("includes an activity line when last_activity_source and last_activity_at are present", () => {
			const workspace: Workspace = {
				...baseWorkspace,
				last_activity_source: "ssh",
				last_activity_at: dayjs().subtract(15, "minute").utc().format(),
			};

			const { tooltip } = autostopDisplay(workspace, "inactive", template);
			const html = renderToStaticMarkup(tooltip);

			expect(html).toContain("Activity detected from SSH");
		});

		it("omits the activity line when last_activity_source and last_activity_at are absent", () => {
			const { tooltip } = autostopDisplay(baseWorkspace, "inactive", template);
			const html = renderToStaticMarkup(tooltip);

			expect(html).not.toContain("Activity detected from");
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
