import { describe, expect, it } from "vitest";
import {
	getGroupMembersAISpendQueryKey,
	isGroupMembersAISpendQueryKey,
} from "./groups";

describe("isGroupMembersAISpendQueryKey", () => {
	it("matches only group member spend queries containing the user", () => {
		const userId = "user-1";

		expect(
			isGroupMembersAISpendQueryKey(
				getGroupMembersAISpendQueryKey("group-1", ["user-2", userId]),
				userId,
			),
		).toBe(true);
		expect(
			isGroupMembersAISpendQueryKey(
				getGroupMembersAISpendQueryKey("group-1", ["user-2"]),
				userId,
			),
		).toBe(false);
		expect(isGroupMembersAISpendQueryKey(["group", "group-1"], userId)).toBe(
			false,
		);
	});
});
