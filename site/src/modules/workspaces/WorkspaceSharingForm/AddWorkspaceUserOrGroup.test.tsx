import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { MockOrganizationMember, MockWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AddWorkspaceUserOrGroup } from "./AddWorkspaceUserOrGroup";

const renderAndSelectMember = async (isLoading: boolean) => {
	vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([]);
	vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
		members: [MockOrganizationMember],
		count: 1,
	});
	const onSubmit = vi.fn((_value: unknown, _role: unknown, reset: () => void) =>
		reset(),
	);
	render(
		<AddWorkspaceUserOrGroup
			organizationID={MockWorkspace.organization_id}
			isLoading={isLoading}
			workspaceACL={{ users: [], group: [] }}
			onSubmit={onSubmit}
		/>,
	);

	const user = userEvent.setup();
	await user.click(
		screen.getByRole("button", { name: "Search for user or group" }),
	);
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(MockOrganizationMember.username),
		}),
	);
	return { user, onSubmit };
};

describe("AddWorkspaceUserOrGroup", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("moves focus to the member search after adding a member", async () => {
		const { user, onSubmit } = await renderAndSelectMember(false);
		await user.click(screen.getByRole("button", { name: "Add member" }));

		// Form actions run in a transition, so the reset lands asynchronously.
		const search = await screen.findByRole("button", {
			name: "Search for user or group",
		});
		expect(onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({ user_id: MockOrganizationMember.user_id }),
			"use",
			expect.any(Function),
		);
		expect(search).toHaveFocus();
	});

	it("does not submit again while an add is pending", async () => {
		const { user, onSubmit } = await renderAndSelectMember(true);
		await user.click(screen.getByRole("button", { name: /Add member/ }));

		expect(onSubmit).not.toHaveBeenCalled();
	});
});
