import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { groupsQueryKey } from "#/api/queries/groups";
import { usersKey } from "#/api/queries/users";
import { MockGroup, MockUserOwner, mockApiError } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import UsersPage from "./UsersPage";

const memberships = [{ ...MockGroup, members: [MockUserOwner] }];
const usersQueryKey = usersKey({ limit: 25, offset: 0, q: "" });

beforeEach(() => {
	vi.spyOn(API, "getUsers").mockResolvedValue({
		users: [MockUserOwner],
		count: 1,
	});
});

it("retries failed memberships without refetching successful users", async () => {
	const user = userEvent.setup();
	const getGroups = vi
		.spyOn(API, "getGroups")
		.mockRejectedValueOnce(
			mockApiError({ message: "Unable to load group memberships." }),
		)
		.mockResolvedValueOnce(memberships);
	const { queryClient } = renderWithAuth(<UsersPage />);

	await user.click(await screen.findByRole("button", { name: "Retry groups" }));

	await waitFor(() => {
		expect(queryClient.getQueryState(groupsQueryKey)?.status).toBe("success");
	});
	expect(getGroups).toHaveBeenCalledTimes(2);
	expect(queryClient.getQueryData(groupsQueryKey)).toEqual(memberships);
	expect(queryClient.getQueryState(usersQueryKey)?.status).toBe("success");
	expect(API.getUsers).toHaveBeenCalledTimes(1);
});

it("retains cached memberships after a failed refetch and retries them", async () => {
	const user = userEvent.setup();
	const getGroups = vi
		.spyOn(API, "getGroups")
		.mockResolvedValueOnce(memberships)
		.mockRejectedValueOnce(
			mockApiError({ message: "Unable to refresh group memberships." }),
		)
		.mockResolvedValueOnce([]);
	const { queryClient } = renderWithAuth(<UsersPage />);
	await waitFor(() => {
		expect(queryClient.getQueryState(groupsQueryKey)?.status).toBe("success");
	});

	await act(async () => {
		await queryClient.invalidateQueries({ queryKey: groupsQueryKey });
	});
	expect(queryClient.getQueryState(groupsQueryKey)?.status).toBe("error");
	expect(queryClient.getQueryData(groupsQueryKey)).toEqual(memberships);
	await user.click(await screen.findByRole("button", { name: "Retry groups" }));

	await waitFor(() => {
		expect(queryClient.getQueryState(groupsQueryKey)?.status).toBe("success");
	});
	expect(getGroups).toHaveBeenCalledTimes(3);
	expect(queryClient.getQueryData(groupsQueryKey)).toEqual([]);
	expect(API.getUsers).toHaveBeenCalledTimes(1);
});
