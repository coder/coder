import { describe, expect, it } from "vitest";
import type { AgentHoursGroupMemberUsage } from "#/api/typesGenerated";
import { agentHoursGroupKind } from "./GroupMemberAgentHoursCells";

const mockGroup = { id: "group-1", organization_id: "org-1" };

const mockUsage: AgentHoursGroupMemberUsage = {
	user_id: "user-1",
	used_ms: 0,
	effective_group: { id: "group-1", name: "group-1", display_name: "" },
};

describe("agentHoursGroupKind", () => {
	it("is everyone for the organization's unallotted share", () => {
		expect(
			agentHoursGroupKind(
				{
					...mockUsage,
					effective_group: { ...mockUsage.effective_group, id: "org-1" },
				},
				mockGroup,
			),
		).toBe("everyone");
	});

	it("is everyone when the viewed group is Everyone itself", () => {
		expect(
			agentHoursGroupKind(
				{
					...mockUsage,
					effective_group: { ...mockUsage.effective_group, id: "org-1" },
				},
				{ id: "org-1", organization_id: "org-1" },
			),
		).toBe("everyone");
	});

	it("is this for the viewed group", () => {
		expect(agentHoursGroupKind(mockUsage, mockGroup)).toBe("this");
	});

	it("is other for any other group", () => {
		expect(
			agentHoursGroupKind(
				{
					...mockUsage,
					effective_group: { ...mockUsage.effective_group, id: "group-2" },
				},
				mockGroup,
			),
		).toBe("otherGroup");
	});
});
