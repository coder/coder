import { describe, expect, it } from "vitest";
import type { AgentHoursGroupMemberUsage } from "#/api/typesGenerated";
import { agentHoursGroupKind } from "./GroupMemberAgentHoursCells";

const group = { id: "group-1", organization_id: "org-1" };

const usageCountingToward = (groupId: string): AgentHoursGroupMemberUsage => ({
	user_id: "user-1",
	used_ms: 0,
	effective_group: { id: groupId, name: "group", display_name: "" },
});

describe("agentHoursGroupKind", () => {
	it("is everyone for the organization's unallotted share", () => {
		expect(agentHoursGroupKind(usageCountingToward("org-1"), group)).toBe(
			"everyone",
		);
	});

	it("is everyone when the viewed group is Everyone itself", () => {
		expect(
			agentHoursGroupKind(usageCountingToward("org-1"), {
				id: "org-1",
				organization_id: "org-1",
			}),
		).toBe("everyone");
	});

	it("is this for the viewed group", () => {
		expect(agentHoursGroupKind(usageCountingToward("group-1"), group)).toBe(
			"this",
		);
	});

	it("is other for any other group", () => {
		expect(agentHoursGroupKind(usageCountingToward("group-2"), group)).toBe(
			"otherGroup",
		);
	});
});
