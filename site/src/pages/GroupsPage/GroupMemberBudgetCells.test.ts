import { describe, expect, it } from "vitest";
import type { GroupMemberAISpend } from "#/api/typesGenerated";
import { effectiveBudgetGroup } from "./GroupMemberBudgetCells";

const group = { id: "group-1", organization_id: "org-1" };

const mockSpend: GroupMemberAISpend = {
	user_id: "user-1",
	effective_group_id: null,
	group_budget: null,
	group_spend_micros: 0,
};

describe("effectiveBudgetGroup", () => {
	it("is none without spend data", () => {
		expect(effectiveBudgetGroup(undefined, group)).toEqual({ kind: "none" });
	});

	it("is other without a governing group (budget in another org)", () => {
		expect(effectiveBudgetGroup(mockSpend, group)).toEqual({ kind: "other" });
	});

	it("is everyone for the org-wide Everyone group", () => {
		expect(
			effectiveBudgetGroup(
				{ ...mockSpend, effective_group_id: "org-1" },
				group,
			),
		).toEqual({ kind: "everyone" });
	});

	it("is everyone when the viewed group is Everyone itself", () => {
		expect(
			effectiveBudgetGroup(
				{ ...mockSpend, effective_group_id: "org-1" },
				{ id: "org-1", organization_id: "org-1" },
			),
		).toEqual({ kind: "everyone" });
	});

	it("is this for the given group", () => {
		expect(
			effectiveBudgetGroup(
				{ ...mockSpend, effective_group_id: "group-1" },
				group,
			),
		).toEqual({ kind: "this" });
	});

	it("is other for any other group", () => {
		expect(
			effectiveBudgetGroup(
				{ ...mockSpend, effective_group_id: "group-2" },
				group,
			),
		).toEqual({ kind: "other" });
	});
});
