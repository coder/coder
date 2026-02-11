import {
	MockDisplayNameTasks,
	MockTask,
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
		spyOn(API, "getTasks").mockReturnValue(new Promise(() => {}));
	},
};

export const Failed: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockRejectedValue(
			mockApiError({
				message: "Failed to fetch tasks",
			}),
		);
	},
};

export const Loaded: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
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
		spyOn(API, "getTasks").mockResolvedValue([]);
	},
};

export const Closed: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: /close sidebar/i });
		await userEvent.click(button);
	},
};

export const OpenOptionsMenu: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
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
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
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

export const PauseMenuOpen: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const optionButtons = await canvas.findAllByRole("button", {
			name: /task options/i,
		});
		await userEvent.click(optionButtons[0]);
	},
};

export const ResumeMenuOpen: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue([
			{ ...MockTask, status: "paused" },
			...MockTasks.slice(1),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const optionButtons = await canvas.findAllByRole("button", {
			name: /task options/i,
		});
		await userEvent.click(optionButtons[0]);
	},
};

export const MixedStatuses: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue([
			MockTask,
			{
				...MockTask,
				id: "paused-task",
				name: "paused-task",
				display_name: "Paused task",
				status: "paused",
			},
			{
				...MockTask,
				id: "error-task",
				name: "error-task",
				display_name: "Error task",
				status: "error",
			},
			{
				...MockTask,
				id: "init-task",
				name: "init-task",
				display_name: "Initializing task",
				status: "initializing",
			},
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const optionButtons = await canvas.findAllByRole("button", {
			name: /task options/i,
		});
		// Open menu on the error task (third item) to show both Pause and Resume.
		await userEvent.click(optionButtons[2]);
	},
};
