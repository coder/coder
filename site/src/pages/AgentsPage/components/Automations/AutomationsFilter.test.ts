import { describe, expect, it } from "vitest";
import type { ChatAutomation } from "#/api/typesGenerated";
import {
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import { filterAutomations } from "./AutomationsFilter";

const schedule: ChatAutomation = {
	...MockChatAutomation,
	id: "schedule",
	name: "Nightly Build",
};
const disabled: ChatAutomation = {
	...MockChatAutomation,
	id: "disabled",
	name: "Weekly report",
	enabled: false,
};
const paused: ChatAutomation = {
	...MockChatAutomation,
	id: "paused",
	name: "Release notes",
	paused_reasons: ["model_unavailable"],
};
const listening: ChatAutomation = {
	...MockWebhookChatAutomation,
	id: "listening",
	name: "PR opened",
	enabled: true,
	webhook_use: "multi",
	webhook_consumed_at: undefined,
};
// The server keeps a used single-use webhook enabled.
const used: ChatAutomation = {
	...listening,
	id: "used",
	name: "Deploy finished",
	webhook_use: "single",
	webhook_consumed_at: "2026-09-29T10:00:00Z",
};
const all = [schedule, disabled, paused, listening, used];

const ids = (query: string) =>
	filterAutomations(all, query).map((automation) => automation.id);

describe("filterAutomations", () => {
	it("keeps every automation for an empty query or an owner alone", () => {
		expect(ids("")).toEqual(all.map((automation) => automation.id));
		expect(ids("owner:alice")).toEqual(all.map((automation) => automation.id));
	});

	it("filters by trigger", () => {
		expect(ids("trigger:webhook")).toEqual(["listening", "used"]);
		expect(ids("trigger:schedule")).toEqual(["schedule", "disabled", "paused"]);
	});

	it("filters by the status the row shows", () => {
		expect(ids("status:enabled")).toEqual(["schedule", "listening"]);
		expect(ids("status:disabled")).toEqual(["disabled"]);
		expect(ids("status:paused")).toEqual(["paused"]);
		// A used single-use webhook counts only as used, though it is enabled.
		expect(ids("status:used")).toEqual(["used"]);
	});

	it("matches the name without regard to case", () => {
		expect(ids("nightly")).toEqual(["schedule"]);
		expect(ids("owner:me status:enabled BUILD")).toEqual(["schedule"]);
		expect(ids("trigger:webhook build")).toEqual([]);
	});
});
