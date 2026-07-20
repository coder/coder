import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import type { User } from "#/api/typesGenerated";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockOrganizationMember,
	MockOrganizationMember2,
	MockOwnerRole,
	MockUserAdminRole,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const meta: Meta<typeof OrganizationMembersPageView> = {
	title: "pages/OrganizationMembersPageView",
	component: OrganizationMembersPageView,
	args: {
		error: undefined,
		filterProps: {
			filter: {
				query: "",
				values: {},
				update: () => {},
				debounceUpdate: () => {},
				cancelDebounce: () => {},
				used: false,
			},
		},
		organizationName: "friends",
		membersQuery: {
			...mockSuccessResult,
			totalRecords: 2,
		} as UsePaginatedQueryResult,
		members: [
			{
				...MockOrganizationMember,
				global_roles: [MockOwnerRole, MockUserAdminRole],
				groups: [],
			},
			{ ...MockOrganizationMember2, groups: [] },
		],
		addMembers: () => Promise.resolve(),
		onEditMemberRoles: () => Promise.resolve(),
		isUpdatingMemberRoles: false,
		removeMember: () => Promise.resolve(),
		me: MockUserOwner.id,
		canEditMembers: true,
		canViewMembers: true,
		canViewActivity: false,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationMembersPageView>;

export const Default: Story = {};

export const WithAIAddonColumn: Story = {
	args: {
		showAISeatColumn: true,
	},
};

export const NoMembers: Story = {
	args: {
		members: [],
	},
};

export const WithError: Story = {
	args: {
		error: "Something went wrong",
	},
};

export const NoEdit: Story = {
	args: {
		canEditMembers: false,
	},
};

export const UpdatingMember: Story = {
	args: {
		isUpdatingMemberRoles: true,
	},
};

// The add-members dialog caps a single selection at 100 users, mirroring the
// server-side max=100 constraint on AddOrganizationMembersRequest.UserIDs. This
// story exercises the boundary: it selects exactly 100 users, verifies the
// cap warning and counter render, and confirms a 101st selection is ignored.
const SELECTION_CAP = 100;

// Clone a real MockUser into a unique roster large enough to exceed the cap.
// The dialog's user query is mocked below, so ids/usernames/emails only need
// to be unique within this list.
const bulkUsers: User[] = Array.from({ length: SELECTION_CAP + 1 }, (_, i) => ({
	...MockUserMember,
	id: `bulk-user-${i}`,
	username: `bulk-user-${i}`,
	email: `bulk-user-${i}@coder.com`,
}));

export const AddUsersSelectionCap: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockResolvedValue({
			users: bulkUsers,
			count: bulkUsers.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		// Open the add-members dialog.
		await userEvent.click(
			await canvas.findByRole("button", { name: "Add users" }),
		);
		const dialog = within(await body.findByRole("dialog"));

		// Wait for the mocked roster to render before interacting with it.
		await dialog.findByLabelText(`Select user ${bulkUsers[0].username}`);

		// Select exactly the cap of 100 users.
		for (let i = 0; i < SELECTION_CAP; i++) {
			await userEvent.click(
				dialog.getByLabelText(`Select user ${bulkUsers[i].username}`),
			);
		}

		// The counter and cap warning both reflect that the limit is reached.
		await dialog.findByText(`${SELECTION_CAP} of ${SELECTION_CAP} selected`);
		await dialog.findByText(
			`You can add up to ${SELECTION_CAP} users at a time.`,
		);

		// At exactly the cap the submit button stays enabled.
		const submit = dialog.getByRole("button", { name: "Add users" });
		expect(submit).toBeEnabled();

		// Attempting to select a 101st user is ignored: the checkbox stays
		// unchecked and the selection count remains at the cap.
		const overflowCheckbox = dialog.getByLabelText(
			`Select user ${bulkUsers[SELECTION_CAP].username}`,
		);
		await userEvent.click(overflowCheckbox);
		expect(overflowCheckbox).not.toBeChecked();
		await dialog.findByText(`${SELECTION_CAP} of ${SELECTION_CAP} selected`);
	},
};
