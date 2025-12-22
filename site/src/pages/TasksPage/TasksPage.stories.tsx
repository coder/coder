import {
	MockDisplayNameTasks,
	MockInitializingTasks,
	MockSystemNotificationTemplates,
	MockTasks,
	MockTemplate,
	MockUserOwner,
	mockApiError,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import {
	getTemplatesQueryKey,
	templateVersionExternalAuth,
	templateVersionPresets,
} from "api/queries/templates";
import { usersKey } from "api/queries/users";
import { MockUsers } from "pages/UsersPage/storybookData/users";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import TasksPage from "./TasksPage";

const meta: Meta<typeof TasksPage> = {
	title: "pages/TasksPage",
	component: TasksPage,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		user: MockUserOwner,
		permissions: {
			viewDeploymentConfig: true,
		},
		queries: [
			{
				key: getTemplatesQueryKey({ q: "has-ai-task:true" }),
				data: [MockTemplate],
			},
			{
				key: usersKey({ q: "", limit: 25, offset: 0 }),
				data: { users: MockUsers, count: MockUsers.length },
			},
			{
				key: templateVersionExternalAuth(MockTemplate.active_version_id)
					.queryKey,
				data: [],
			},
			{
				key: templateVersionPresets(MockTemplate.active_version_id).queryKey,
				data: null,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
	},
};

export default meta;
type Story = StoryObj<typeof TasksPage>;

export const LoadingTemplates: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockImplementation(
			() => new Promise(() => 1000 * 60 * 60),
		);
	},
};

export const EmptyTemplates: Story = {
	parameters: {
		queries: [
			{
				key: ["templates", { q: "has-ai-task:true" }],
				data: [],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue([]);
	},
};

export const LoadingTemplatesError: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplates").mockRejectedValue(
			mockApiError({
				message: "Failed to load AI templates",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const LoadingTasks: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockImplementation(
			() => new Promise(() => 1000 * 60 * 60),
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Select the first AI template", async () => {
			const form = await canvas.findByRole("form");
			const combobox = await within(form).findByRole("combobox");
			expect(combobox).toHaveTextContent(MockTemplate.display_name);
		});
	},
};

export const LoadingTasksError: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockRejectedValue(
			mockApiError({
				message: "Failed to load tasks",
			}),
		);
	},
};

export const EmptyTasks: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue([]);
	},
};

export const LoadedTasks: Story = {};

export const DisplayName: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockDisplayNameTasks);
	},
};

export const LoadedTasksWaitingForInputTab: Story = {
	beforeEach: () => {
		const [firstTask, ...otherTasks] = MockTasks;
		spyOn(API, "getTasks").mockResolvedValue([
			{
				...firstTask,
				id: "active-idle-task",
				display_name: "Active Idle Task",
				status: "active",
				current_state: {
					...firstTask.current_state,
					state: "idle",
				},
			},
			{
				...firstTask,
				id: "paused-idle-task",
				display_name: "Paused Idle Task",
				status: "paused",
				current_state: {
					...firstTask.current_state,
					state: "idle",
				},
			},
			{
				...firstTask,
				id: "error-idle-task",
				display_name: "Error Idle Task",
				status: "error",
				current_state: {
					...firstTask.current_state,
					state: "idle",
				},
			},
			...otherTasks,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Switch to 'Waiting for input' tab", async () => {
			const waitingForInputTab = await canvas.findByRole("button", {
				name: /waiting for input/i,
			});
			await userEvent.click(waitingForInputTab);

			// Wait for the table to update after tab switch
			await waitFor(async () => {
				const table = canvas.getByRole("table");
				const tableContent = within(table);

				// Active idle task should be visible
				expect(tableContent.getByText("Active Idle Task")).toBeInTheDocument();

				// Only active idle tasks should be visible in the table
				expect(
					tableContent.queryByText("Paused Idle Task"),
				).not.toBeInTheDocument();
				expect(
					tableContent.queryByText("Error Idle Task"),
				).not.toBeInTheDocument();
			});
		});
	},
};

export const NonAdmin: Story = {
	parameters: {
		permissions: {
			viewDeploymentConfig: false,
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Can't see filters", async () => {
			await canvas.findByRole("table");
			expect(
				canvas.queryByRole("region", { name: /filters/i }),
			).not.toBeInTheDocument();
		});
	},
};

export const OpenDeleteDialog: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const deleteButtons = await canvas.findAllByRole("button", {
			name: /delete task/i,
		});
		await userEvent.click(deleteButtons[0]);
	},
};

export const InitializingTasks: Story = {
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockInitializingTasks);
	},
};

export const BatchActionsEnabled: Story = {
	parameters: {
		features: ["task_batch_actions"],
	},
};

export const BatchActionsSomeSelected: Story = {
	parameters: {
		features: ["task_batch_actions"],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Select first two tasks", async () => {
			await canvas.findByRole("table");
			const checkboxes = await canvas.findAllByRole("checkbox");
			// Skip the "select all" checkbox (first one) and select the next two
			await userEvent.click(checkboxes[1]);
			await userEvent.click(checkboxes[2]);
		});
	},
};

export const BatchActionsAllSelected: Story = {
	parameters: {
		features: ["task_batch_actions"],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Select all tasks using header checkbox", async () => {
			await canvas.findByRole("table");
			const checkboxes = await canvas.findAllByRole("checkbox");
			// Click the first checkbox (select all)
			await userEvent.click(checkboxes[0]);
		});
	},
};

export const BatchActionsDropdownOpen: Story = {
	parameters: {
		features: ["task_batch_actions"],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Select some tasks", async () => {
			await canvas.findByRole("table");
			const checkboxes = await canvas.findAllByRole("checkbox");
			await userEvent.click(checkboxes[1]);
			await userEvent.click(checkboxes[2]);
		});

		await step("Open bulk actions dropdown", async () => {
			const bulkActionsButton = await canvas.findByRole("button", {
				name: /bulk actions/i,
			});
			await userEvent.click(bulkActionsButton);
		});
	},
};

export const AllTaskNotificationsDisabledAlertVisible: Story = {
	parameters: {
		queries: [
			{
				// User notification preferences: empty because user hasn't changed defaults
				// Task notifications are disabled by default (enabled_by_default: false)
				key: ["users", MockUserOwner.id, "notifications", "preferences"],
				data: [],
			},
			{
				// System notification templates: includes task notifications with enabled_by_default: false
				key: ["notifications", "templates", "system"],
				data: MockSystemNotificationTemplates,
			},
			{
				// User preferences: alert NOT dismissed
				key: ["me", "preferences"],
				data: { task_notification_alert_dismissed: false },
			},
		],
	},
};

export const AllTaskNotificationsDisabledAlertDismissed: Story = {
	parameters: {
		queries: [
			{
				// User notification preferences: empty because user hasn't changed defaults
				// Task notifications are disabled by default (enabled_by_default: false)
				key: ["users", MockUserOwner.id, "notifications", "preferences"],
				data: [],
			},
			{
				// System notification templates: includes task notifications with enabled_by_default: false
				key: ["notifications", "templates", "system"],
				data: MockSystemNotificationTemplates,
			},
			{
				// User preferences: alert IS dismissed
				key: ["me", "preferences"],
				data: { task_notification_alert_dismissed: true },
			},
		],
	},
};

export const OneTaskNotificationEnabledAlertHidden: Story = {
	parameters: {
		queries: [
			{
				// User has explicitly enabled one task notification (Task Working)
				// Since at least one task notification is enabled, the warning alert should not appear
				key: ["users", MockUserOwner.id, "notifications", "preferences"],
				data: [
					{
						id: "bd4b7168-d05e-4e19-ad0f-3593b77aa90f", // Task Working
						disabled: false,
						updated_at: new Date().toISOString(),
					},
				],
			},
			{
				// System notification templates: includes task notifications with enabled_by_default: false
				key: ["notifications", "templates", "system"],
				data: MockSystemNotificationTemplates,
			},
			{
				// User preferences: doesn't matter since alert shouldn't show anyway
				key: ["me", "preferences"],
				data: { task_notification_alert_dismissed: false },
			},
		],
	},
};

export const AllTaskNotificationsExplicitlyDisabledAlertVisible: Story = {
	parameters: {
		queries: [
			{
				// User has explicitly disabled a task notification
				key: ["users", MockUserOwner.id, "notifications", "preferences"],
				data: [
					{
						id: "d4a6271c-cced-4ed0-84ad-afd02a9c7799", // Task Idle
						disabled: true,
						updated_at: "2024-08-06T11:58:37.755053Z",
					},
				],
			},
			{
				// System notification templates: includes task notifications with enabled_by_default: false
				key: ["notifications", "templates", "system"],
				data: MockSystemNotificationTemplates,
			},
			{
				// User preferences: alert NOT dismissed
				key: ["me", "preferences"],
				data: { task_notification_alert_dismissed: false },
			},
		],
	},
};
