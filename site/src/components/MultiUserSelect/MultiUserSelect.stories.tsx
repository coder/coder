import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { usersKey } from "api/queries/users";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { spyOn } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { MultiUserSelect } from "./MultiUserSelect";

const meta: Meta<typeof MultiUserSelect> = {
	title: "components/MultiUserSelect",
	component: MultiUserSelect,
};

export default meta;
type Story = StoryObj<typeof MultiUserSelect>;

export const Loading: Story = {
	parameters: {
		queries: [
			{
				key: usersKey({ limit: 25, q: "" }),
				data: {
					users: undefined,
					count: 0,
				},
			},
		],
	},
};

export const WithError: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockRejectedValue(
			mockApiError({
				message: "Failed to load users",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
	args: {
		selected: [],
		onChange: () => undefined,
	},
};

export const Loaded: Story = {
	args: {
		selected: [MockUsers[0], MockUsers[5]],
		onChange: () => undefined,
	},
	parameters: {
		queries: [
			{
				key: usersKey({ limit: 25, q: "" }),
				data: {
					users: MockUsers,
					count: MockUsers.length,
				},
			},
		],
	},
};

export const NoUsers: Story = {
	args: {
		selected: [],
		onChange: () => undefined,
	},
	parameters: {
		queries: [
			{
				key: usersKey({ limit: 25, q: "" }),
				data: {
					users: [],
					count: 0,
				},
			},
		],
	},
};

const filteredUsers = MockUsers.filter((u) =>
	u.username.toLowerCase().includes("andrew"),
);

export const FilterMatch: Story = {
	args: {
		filter: "andrew",
		selected: [],
		onChange: () => undefined,
	},
	parameters: {
		queries: [
			{
				key: usersKey({ limit: 25, q: "andrew" }),
				data: {
					users: filteredUsers,
					count: filteredUsers.length,
				},
			},
		],
	},
};

export const FilterNoMatch: Story = {
	args: {
		filter: "nonexistent",
		selected: [],
		onChange: () => undefined,
	},
	parameters: {
		queries: [
			{
				key: usersKey({ limit: 25, q: "" }),
				data: {
					users: MockUsers,
					count: MockUsers.length,
				},
			},
			{
				key: usersKey({ limit: 25, q: "nonexistent" }),
				data: {
					users: [],
					count: 0,
				},
			},
		],
	},
};
