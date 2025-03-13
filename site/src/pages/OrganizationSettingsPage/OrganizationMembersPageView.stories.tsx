import type { Meta, StoryObj } from "@storybook/react";
import {
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUser,
} from "testHelpers/entities";
import { OrganizationMembersPageView } from "./OrganizationMembersPageView";
import { UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import { mockSuccessResult } from "components/PaginationWidget/PaginationContainer.mocks";

const meta: Meta<typeof OrganizationMembersPageView> = {
	title: "pages/OrganizationMembersPageView",
	component: OrganizationMembersPageView,
	args: {
		canEditMembers: true,
		error: undefined,
		isAddingMember: false,
		isUpdatingMemberRoles: false,
		canViewMembers: true,
		me: MockUser,
		members: [
			{ ...MockOrganizationMember, groups: [] },
			{ ...MockOrganizationMember2, groups: [] },
		],
		membersQuery: {
			...mockSuccessResult,
			totalRecords: 2,
		} as UsePaginatedQueryResult,
		addMember: () => Promise.resolve(),
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
