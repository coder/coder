import {
	MockDisplayNameTasks,
	MockTasks,
	MockUserOwner,
	mockApiError,
} from "testHelpers/entities";
import { withAuthProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { TasksSidebar } from "./TasksSidebar";

const meta: Meta<typeof TasksSidebar> = {
	title: "modules/tasks/TasksSidebar",
	component: TasksSidebar,
	decorators: [withAuthProvider],
	parameters: {
		user: MockUserOwner,
		layout: "fullscreen",
		permissions: {
			viewAllUsers: true,
		},
		reactRouter: reactRouterParameters({
			location: {
				path: `/tasks/${MockTasks[0].owner_name}/${MockTasks[0].id}`,
				pathParams: {
					owner_name: MockTasks[0].owner_name,
					taskId: MockTasks[0].id,
				},
			},
			routing: [
				{ path: "/tasks/:username/:taskId", useStoryElement: true },
				{ path: "/tasks", element: <div>Tasks Index Page</div> },
			],
		}),
	},
	beforeEach: () => {
		spyOn(API, "getUsers").mockResolvedValue({
			users: MockUsers,
			count: MockUsers.length,
		});
	},
};

export default meta;
type Story = StoryObj<typeof TasksSidebar>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockReturnValue(new Promise(() => {}));
	},
};

export const Failed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockRejectedValue(
			mockApiError({
				message: "Failed to fetch tasks",
			}),
		);
	},
};

export const Loaded: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue(MockTasks);
	},
};

export const DisplayName: Story = {
	parameters: {
		queries: [
			{
				key: ["tasks", { owner: MockUserOwner.username }],
				data: MockDisplayNameTasks,
			},
		],
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue([]);
	},
};

export const Closed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue(MockTasks);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: /close sidebar/i });
		await userEvent.click(button);
	},
};

export const OpenOptionsMenu: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue(MockTasks);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const optionButtons = await canvas.findAllByRole("button", {
			name: /task options/i,
		});
		await userEvent.click(optionButtons[0]);
	},
};

export const OpenDeleteDialog: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue(MockTasks);
	},
	play: async ({ canvasElement, step }) => {
		await step("Open menu", async () => {
			const canvas = within(canvasElement);
			const optionButtons = await canvas.findAllByRole("button", {
				name: /task options/i,
			});
			await userEvent.click(optionButtons[0]);
		});
		await step("Open delete dialog", async () => {
			const body = within(canvasElement.ownerDocument.body);
			const deleteButton = await body.findByRole("menuitem", {
				name: /delete/i,
			});
			await userEvent.click(deleteButton);
		});
	},
};
