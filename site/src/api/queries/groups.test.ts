import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import {
	getGroupMembersAISpendQueryKey,
	invalidateGroupMembersAISpend,
} from "./groups";

describe("invalidateGroupMembersAISpend", () => {
	it("invalidates only group member spend queries containing the user", async () => {
		const queryClient = new QueryClient();
		const userId = "user-1";
		const spendWithUser = getGroupMembersAISpendQueryKey("group-1", [
			"user-2",
			userId,
		]);
		const spendWithoutUser = getGroupMembersAISpendQueryKey("group-2", [
			"user-2",
		]);
		const otherGroupQuery = ["group", "group-1"];

		queryClient.setQueryData(spendWithUser, {});
		queryClient.setQueryData(spendWithoutUser, {});
		queryClient.setQueryData(otherGroupQuery, {});

		await invalidateGroupMembersAISpend(queryClient, userId);

		expect(queryClient.getQueryState(spendWithUser)?.isInvalidated).toBe(true);
		expect(queryClient.getQueryState(spendWithoutUser)?.isInvalidated).toBe(
			false,
		);
		expect(queryClient.getQueryState(otherGroupQuery)?.isInvalidated).toBe(
			false,
		);
	});
});
