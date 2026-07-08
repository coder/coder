import { describe, expect, it } from "vitest";
import type { GroupMemberAICostControl } from "#/api/api";
import { effectiveBudgetGroup } from "./GroupMemberBudgetCells";

const group = { id: "group-1", organization_id: "org-1" };

const costControl = (
	effectiveGroupId: string | null,
): GroupMemberAICostControl => ({
	current_spend_micros: 0,
	spend_limit_micros: null,
	effective_group_id: effectiveGroupId,
	limit_source: "group",
});

describe("effectiveBudgetGroup", () => {
	it("is none without cost control data", () => {
		expect(effectiveBudgetGroup(undefined, group)).toEqual({ kind: "none" });
	});

	it("is none without a governing group", () => {
		expect(effectiveBudgetGroup(costControl(null), group)).toEqual({
			kind: "none",
		});
	});

	it("is everyone for the org-wide Everyone group", () => {
		expect(effectiveBudgetGroup(costControl("org-1"), group)).toEqual({
			kind: "everyone",
		});
	});

	it("is everyone when the viewed group is Everyone itself", () => {
		expect(
			effectiveBudgetGroup(costControl("org-1"), {
				id: "org-1",
				organization_id: "org-1",
			}),
		).toEqual({ kind: "everyone" });
	});

	it("is this for the given group", () => {
		expect(effectiveBudgetGroup(costControl("group-1"), group)).toEqual({
			kind: "this",
		});
	});

	it("is other for any other group", () => {
		expect(effectiveBudgetGroup(costControl("group-2"), group)).toEqual({
			kind: "other",
			groupId: "group-2",
		});
	});
});
