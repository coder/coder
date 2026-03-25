import {
	MockOrganizationMember,
	MockOrganizationMember2,
	MockOwnerRole,
	MockUserAdminRole,
	MockUserOwner,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import type { ComponentProps } from "react";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";

type FilterProps = ComponentProps<
	typeof OrganizationMembersPageView
>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	values: {
		status: "active",
	},
	menus: {
		status: MockMenu,
	},
});

const meta: Meta<typeof OrganizationMembersPageView> = {
	title: "pages/OrganizationMembersPageView",
	component: OrganizationMembersPageView,
	args: {
		filterProps: defaultFilterProps,
		canEditMembers: true,
		error: undefined,
		isAddingMember: false,
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

export const AddingMember: Story = {
	args: {
		isAddingMember: true,
	},
};

export const UpdatingMember: Story = {
	args: {
		isUpdatingMemberRoles: true,
	},
};
