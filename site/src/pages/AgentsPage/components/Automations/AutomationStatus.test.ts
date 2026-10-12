import { describe, expect, it } from "vitest";
import { MockChatAutomation } from "#/testHelpers/chatEntities";
import { scheduleInactiveNotice } from "./AutomationStatus";

describe("scheduleInactiveNotice", () => {
	it("says the runs of a turned-off automation will not happen", () => {
		expect(
			scheduleInactiveNotice({ ...MockChatAutomation, enabled: false }),
		).toBe(
			"The automation is off, so these runs will not happen until it is turned on.",
		);
	});

	it("says why a paused automation will not run", () => {
		expect(
			scheduleInactiveNotice({
				...MockChatAutomation,
				paused_reasons: ["target_unavailable"],
			}),
		).toBe(
			"The automation is paused, so these runs will not happen. Its chat is archived or deleted. Unarchive the chat, or create a new automation.",
		);
	});

	it("says nothing for an automation that will run", () => {
		expect(scheduleInactiveNotice(MockChatAutomation)).toBeUndefined();
	});
});
