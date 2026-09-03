import { describe, expect, it } from "vitest";
import type { GroupMemberAISpend } from "#/api/typesGenerated";
import { effectiveBudgetGroup } from "./GroupMemberBudgetCells";

const group = { id: "group-1", organization_id: "org-1" };

const mockSpend: GroupMemberAISpend = {
	user_id: "user-1",
	effective_group_id: null,
	effective_budget: null,
	group_budget: null,
	group_spend_micros: 0,
};

describe("effectiveBudgetGroup", () => {
	it("is none without spend data", () => {
		expect(effectiveBudgetGroup(undefined, group)).toEqual({ kind: "none" });
	});

	it("is other organization when the effective group is masked", () => {
		expect(effectiveBudgetGroup(mockSpend, group)).toEqual({
			kind: "otherOrg",
		});
	});

	it("is everyone for the unlimited Everyone fallback", () => {
		expect(
			effectiveBudgetGroup(
				{ ...mockSpend, effective_group_id: "org-1" },
				group,
			),
		).toEqual({ kind: "everyone" });
	});

	it("is other for a budgeted Everyone group viewed from a regular group", () => {
		expect(
			effectiveBudgetGroup(
				{
					...mockSpend,
					effective_group_id: "org-1",
					effective_budget: {
						spend_limit_micros: 1_000_000,
						limit_source: "group",
					},
				},
				group,
			),
		).toEqual({ kind: "otherGroup" });
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
		).toEqual({ kind: "otherGroup" });
	});
});
