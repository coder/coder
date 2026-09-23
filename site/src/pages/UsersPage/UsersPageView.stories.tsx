import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, fn, within } from "storybook/test";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { UsersPageView } from "./UsersPageView";

type FilterProps = ComponentProps<typeof UsersPageView>["filterProps"];

const defaultFilterProps: FilterProps = {
	...getDefaultFilterProps<FilterProps>({
		query: "status:active",
		values: { status: "active" },
	}),
	lastSeen: {
		start: new Date(0),
		end: new Date("2026-03-12T12:00:00Z"),
		preset: "all_time",
	},
	onLastSeenChange: fn(),
};

const meta: Meta<typeof UsersPageView> = {
	title: "pages/UsersPageView",
	component: UsersPageView,
	args: {
		canEditUsers: true,
		me: MockUserOwner.id,
		filterProps: defaultFilterProps,
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 2,
			data: {
				count: 2,
				users: [MockUserOwner, MockUserMember],
			},
		},
	},
};

export default meta;
type Story = StoryObj<typeof UsersPageView>;

export const Admin: Story = {
	args: {
		canCreateUser: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("link", { name: "New user" })).toBeVisible();
	},
};

export const SmallViewport: Story = {
	parameters: {
		pixel: { matrix: { viewports: ["tablet"] } },
	},
};

export const Member: Story = {
	args: { canEditUsers: false },
};

export const FilteredByStatusRoleAndType: Story = {
	args: {
		filterProps: {
			...defaultFilterProps,
			filter: {
				...defaultFilterProps.filter,
				query: "status:active role:owner service_account:false",
			},
		},
	},
};

export const LastSeenRange: Story = {
	args: {
		filterProps: {
			...defaultFilterProps,
			lastSeen: {
				start: new Date("2026-03-05T12:00:00Z"),
				end: new Date("2026-03-12T12:00:00Z"),
				preset: "last_7d",
			},
		},
	},
};

export const Empty: Story = {
	args: {
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				count: 0,
				users: [],
			},
		},
	},
};

export const EmptyPage: Story = {
	args: {
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				count: 0,
				users: [],
			},
		},
	},
};

export const WithError: Story = {
	args: {
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: {
				count: 0,
				users: [],
			},
		},
		filterProps: {
			...defaultFilterProps,
			error: mockApiError({
				message: "Invalid user search query.",
				validations: [
					{
						field: "status",
						detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
					},
				],
			}),
		},
	},
};
