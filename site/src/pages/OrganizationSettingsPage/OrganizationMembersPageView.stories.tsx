import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	MockOrganizationMember,
	MockOrganizationMember2,
	MockOwnerRole,
	MockUserAdminRole,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
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

export const Loading: Story = {
	args: {
		members: undefined,
	},
};

export const NoMembers: Story = {
	args: {
		members: [],
		membersQuery: { ...mockSuccessResult, totalRecords: 0 },
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

export const FiltersOpen: Story = {
	parameters: { pixel: { matrix: pixelWithTablet } },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};

export const SelectedFilters: Story = {
	args: {
		filterProps: getDefaultFilterProps({
			query: "service_account:true",
			values: { service_account: "true" },
		}),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};

export const InvalidFilter: Story = {
	args: {
		error: mockApiError({
			message: "Invalid member search query.",
			validations: [
				{ field: "status", detail: "Status must be a valid user status." },
			],
		}),
	},
};
