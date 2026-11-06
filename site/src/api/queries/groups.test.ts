import { QueryClient, QueryObserver } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockGroup, MockUserOwner } from "#/testHelpers/entities";
import {
	getGroupMembersAISpendQueryKey,
	groupsByUserId,
	invalidateGroupMembersAISpend,
} from "./groups";
import { users } from "./users";

describe("groupsByUserId", () => {
	it.each([
		{ name: "empty", memberships: [] },
		{
			name: "populated",
			memberships: [{ ...MockGroup, members: [MockUserOwner] }],
		},
	])(
		"keeps successful users independent of group errors and retries $name memberships",
		async ({ memberships }) => {
			const queryClient = new QueryClient({
				defaultOptions: { queries: { retry: false } },
			});
			const error = new Error("Unable to load group memberships.");
			const getGroups = vi
				.spyOn(API, "getGroups")
				.mockRejectedValueOnce(error)
				.mockResolvedValueOnce(memberships);
			vi.spyOn(API, "getUsers").mockResolvedValue({
				users: [MockUserOwner],
				count: 1,
			});
			const usersQuery = users({ limit: 25, offset: 0, q: "" });
			const observer = new QueryObserver(queryClient, groupsByUserId());

			await queryClient.fetchQuery(usersQuery);
			const failed = await observer.refetch();
			expect(failed).toMatchObject({ status: "error", data: undefined, error });
			expect(queryClient.getQueryState(usersQuery.queryKey)?.status).toBe(
				"success",
			);

			const retried = await observer.refetch();
			expect(getGroups).toHaveBeenCalledTimes(2);
			expect(retried.status).toBe("success");
			expect(retried.data?.get(MockUserOwner.id) ?? []).toEqual(memberships);
			queryClient.clear();
		},
	);

	it("preserves selected memberships when a background refetch fails", async () => {
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		const memberships = [{ ...MockGroup, members: [MockUserOwner] }];
		const query = groupsByUserId();
		queryClient.setQueryData(query.queryKey, memberships);
		vi.spyOn(API, "getGroups").mockRejectedValue(
			new Error("Unable to load groups."),
		);
		const observer = new QueryObserver(queryClient, query);
		const cached = observer.getCurrentResult().data;

		const failed = await observer.refetch();
		expect(failed.status).toBe("error");
		expect(failed.data).toBe(cached);
		expect(failed.data?.get(MockUserOwner.id)).toEqual(memberships);
		queryClient.clear();
	});
});

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
