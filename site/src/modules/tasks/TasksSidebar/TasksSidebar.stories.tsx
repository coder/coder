import { MockTasks, MockUserOwner, mockApiError } from "testHelpers/entities";
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
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					workspace: MockTasks[0].workspace.name,
				},
			},
			routing: { path: "/tasks/:workspace" },
		}),
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
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					workspace: MockTasks[0].workspace.name,
				},
			},
			routing: { path: "/tasks/:workspace" },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", { name: /close sidebar/i });
		await userEvent.click(button);
	},
};
