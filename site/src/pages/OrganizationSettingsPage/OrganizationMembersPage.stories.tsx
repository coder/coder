import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withOrganizationSettingsProvider,
} from "#/testHelpers/storybook";
import OrganizationMembersPage from "./OrganizationMembersPage";

const meta: Meta<typeof OrganizationMembersPage> = {
	title: "pages/OrganizationMembersPage",
	component: OrganizationMembersPage,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withOrganizationSettingsProvider,
	],
	parameters: {
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: {
				pathParams: { organization: MockDefaultOrganization.name },
			},
			routing: { path: "/organizations/:organization/members" },
		}),
	},
	beforeEach: () => {
		spyOn(API, "getOrganizationRoles").mockResolvedValue([]);
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([]);
		spyOn(API, "getUsers").mockResolvedValue({
			users: [MockUserMember, MockUserOwner],
			count: 2,
		});

		// The members list starts with a single member and gains the newly
		// added member once addOrganizationMembers succeeds, so the story can
		// assert the list refreshes after the mutation invalidates its query.
		let added = false;
		spyOn(API, "getOrganizationPaginatedMembers").mockImplementation(
			async () => {
				const members = added
					? [MockOrganizationMember, MockOrganizationMember2]
					: [MockOrganizationMember];
				return { members, count: members.length };
			},
		);
		spyOn(API, "addOrganizationMembers").mockImplementation(async () => {
			added = true;
			return [];
		});
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationMembersPage>;

export const AddMembers: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		// Open the add-members dialog.
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add users" }),
		);

		// Select two users from the dialog.
		const dialog = within(await body.findByRole("dialog"));
		await userEvent.click(
			await dialog.findByLabelText(`Select user ${MockUserMember.username}`),
		);
		await userEvent.click(
			await dialog.findByLabelText(`Select user ${MockUserOwner.username}`),
		);

		// Submit the selection.
		await userEvent.click(dialog.getByRole("button", { name: "Add users" }));

		// The handler maps the selected users to a single batch request.
		await waitFor(() => {
			expect(API.addOrganizationMembers).toHaveBeenCalledTimes(1);
		});
		expect(API.addOrganizationMembers).toHaveBeenCalledWith(
			MockDefaultOrganization.name,
			{ user_ids: [MockUserMember.id, MockUserOwner.id] },
		);

		// The members list refetches and shows the newly added member.
		await canvas.findByText(MockOrganizationMember2.email);
	},
};
