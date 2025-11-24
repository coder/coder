import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { waitFor } from "@testing-library/react";
import { API } from "api/api";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { useState } from "react";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { UserCombobox } from "./UserCombobox";

const meta: Meta<typeof UserCombobox> = {
	title: "modules/tasks/TasksSidebar/UserCombobox",
	component: UserCombobox,
	decorators: [withAuthProvider],
	parameters: {
		user: MockUserOwner,
	},
	render: (args) => {
		const [value, setValue] = useState("");
		return <UserCombobox {...args} value={value} onValueChange={setValue} />;
	},
};

export default meta;
type Story = StoryObj<typeof UserCombobox>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockImplementation(() => {
			return new Promise(() => {
				// never resolves
			});
		});
	},
};

export const Loaded: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockResolvedValue({
			count: MockUsers.length,
			users: MockUsers,
		});
	},
};

export const SelectUser: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockResolvedValue({
			count: MockUsers.length,
			users: MockUsers,
		});
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		await step("open combobox", async () => {
			const trigger = await canvas.findByText(/all users/i, { exact: false });
			await user.click(trigger);
		});

		await step("select user", async () => {
			const option = await body.findByText(MockUsers[1].name!, {
				exact: false,
			});
			await user.click(option);
		});
	},
};

export const SearchUser: Story = {
	beforeEach: () => {
		spyOn(API, "getUsers").mockImplementation((options) => {
			let users = MockUsers;

			if (options.q?.includes("Ivan")) {
				users = users.filter((u) => u.name?.includes("Ivan"));
			}

			return Promise.resolve({
				count: MockUsers.length,
				users: MockUsers,
			});
		});
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		await step("open combobox", async () => {
			const trigger = await canvas.findByText(/all users/i, { exact: false });
			await user.click(trigger);
		});

		await step("search user", async () => {
			const searchInput = await body.findByLabelText("Search user");
			await user.type(searchInput, "Ivan");
			await waitFor(() => {
				expect(API.getUsers).toHaveBeenCalledTimes(2);
			});
		});
	},
};
