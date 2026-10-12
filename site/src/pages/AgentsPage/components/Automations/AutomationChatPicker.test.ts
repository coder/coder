import { describe, expect, it } from "vitest";
import { isAutomationRunChat } from "./AutomationChatPicker";

const automations = [
	{ name: "Daily digest", target_mode: "new_chat" as const },
	{ name: "Release heartbeat", target_mode: "existing_chat" as const },
];

describe("isAutomationRunChat", () => {
	it.each([
		["Daily digest · 12 Oct 04:15 PDT", true],
		// The user's own chat that only shares a word with the automation.
		["Daily digest notes", false],
		// Existing-chat automations never create chats.
		["Release heartbeat · 12 Oct 09:00 UTC", false],
		["Release triage", false],
	])("treats %s as a run chat: %s", (title, expected) => {
		expect(isAutomationRunChat({ title }, automations)).toBe(expected);
	});
});
