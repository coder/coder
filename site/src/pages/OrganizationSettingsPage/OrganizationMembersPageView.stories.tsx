import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockOrganizationMember,
	MockOrganizationMember2,
	MockOwnerRole,
	MockUserAdminRole,
	MockUserOwner,
} from "#/testHelpers/entities";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

const meta: Meta<typeof OrganizationMembersPageView> = {
	title: "pages/OrganizationMembersPageView",
	component: OrganizationMembersPageView,
	args: {
		canEditMembers: true,
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
		isUpdatingMemberRoles: false,
		canViewMembers: true,
		me: MockUserOwner,
		members: [
			{
				...MockOrganizationMember,
				global_roles: [MockOwnerRole, MockUserAdminRole],
				groups: [],
			},
			{ ...MockOrganizationMember2, groups: [] },
		],
		membersQuery: {
			...mockSuccessResult,
			totalRecords: 2,
		} as UsePaginatedQueryResult,
		addMembers: () => Promise.resolve(),
		removeMember: () => Promise.resolve(),
		updateMemberRoles: () => Promise.resolve(),
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationMembersPageView>;

export const Default: Story = {};

export const WithAIAddonColumn: Story = {
	args: {
		showAISeatColumn: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const header = await canvas.findByRole("columnheader", {
			name: /AI add-on/i,
		});

		await expect(header).toBeVisible();
	},
};

export const WithoutAIAddonColumn: Story = {
	args: {
		showAISeatColumn: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("columnheader", { name: "User" });

		await expect(
			canvas.queryByRole("columnheader", { name: /AI add-on/i }),
		).not.toBeInTheDocument();
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
