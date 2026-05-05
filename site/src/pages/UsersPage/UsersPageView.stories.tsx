import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { UsersPageView } from "./UsersPageView";

type FilterProps = ComponentProps<typeof UsersPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "owner:me",
	menus: {
		status: MockMenu,
	},
	values: {
		status: "active",
	},
});

const meta: Meta<typeof UsersPageView> = {
	title: "pages/UsersPageView",
	component: UsersPageView,
	args: {
		canEditUsers: true,
		filterProps: defaultFilterProps,
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 2,
			data: {
				count: 2,
				users: [
					{ ...MockUserOwner, has_ai_seat: false },
					{ ...MockUserMember, has_ai_seat: false },
				],
			},
		},
	},
};

export default meta;
type Story = StoryObj<typeof UsersPageView>;

export const Admin: Story = {};

export const SmallViewport: Story = {
	parameters: {
		chromatic: { viewports: [600] },
	},
};

export const Member: Story = {
	args: { canEditUsers: false },
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
