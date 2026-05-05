import type { Meta, StoryObj } from "@storybook/react-vite";
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
