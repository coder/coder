import { describe, expect, it } from "vitest";
import { isAutomationRunChat } from "./AutomationChatPicker";

describe("isAutomationRunChat", () => {
	it("treats a chat an automation created as a run chat", () => {
		expect(
			isAutomationRunChat({
				automation_id: "6d9c3f0e-0b3e-4e57-9c1e-0f2b8f0f8a11",
			}),
		).toBe(true);
	});

	it("treats a chat without an automation as the user's own chat", () => {
		expect(isAutomationRunChat({ automation_id: undefined })).toBe(false);
	});
});
