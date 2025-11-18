import {
	MockDeletedWorkspace,
	MockFailedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockTask,
	MockTasks,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgentLogSource,
	MockWorkspaceAgentReady,
	MockWorkspaceAgentStartError,
	MockWorkspaceAgentStarting,
	MockWorkspaceAgentStartTimeout,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceResource,
	mockApiError,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withGlobalSnackbar,
	withProxyProvider,
	withWebSocket,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type { Task, Workspace, WorkspaceApp } from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import TaskPage from "./TaskPage";

const MockClaudeCodeApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "claude-code",
	display_name: "Claude Code",
	slug: "claude-code",
	icon: "/icon/claude.svg",
	health: "healthy",
	healthcheck: {
		url: "http://localhost:3000/health",
		interval: 10,
		threshold: 3,
	},
	statuses: [
		MockWorkspaceAppStatus,
		{
			...MockWorkspaceAppStatus,
			id: "2",
			message: "Planning changes",
			state: "working",
		},
	],
};

const MockVSCodeApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "vscode",
	slug: "vscode",
	display_name: "VS Code Web",
	icon: "/icon/code.svg",
	health: "healthy",
};

const meta: Meta<typeof TaskPage> = {
	title: "pages/TaskPage",
	component: TaskPage,
	decorators: [withProxyProvider(), withAuthProvider],
	beforeEach: () => {
		spyOn(API.experimental, "getTasks").mockResolvedValue(MockTasks);
	},
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: MockTask.owner_name,
					taskId: MockTask.id,
				},
			},
			routing: { path: "/tasks/:username/:taskId" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof TaskPage>;

export const LoadingTask: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async () => {
		await waitFor(() => {
			expect(API.experimental.getTask).toHaveBeenCalledWith(
				MockTask.owner_name,
				MockTask.id,
			);
		});
	},
};

export const LoadingWorkspace: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};

export const LoadingTaskError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockRejectedValue(
			mockApiError({
				message: "Failed to load task",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const LoadingWorkspaceError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockRejectedValue(
			mockApiError({
				message: "Failed to load workspace",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const WaitingOnBuild: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStartingWorkspace,
		);
	},
};

export const FailedBuild: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockFailedWorkspace,
		);
	},
};

export const TerminatedBuild: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
	},
};

export const TerminatedBuildWithStatus: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue({
			...MockStoppedWorkspace,
			latest_app_status: MockWorkspaceAppStatus,
		});
	},
};

export const DeletedWorkspace: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockDeletedWorkspace,
		);
	},
};

export const WaitingStartupScripts: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue({
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				has_ai_task: true,
				resources: [
					{ ...MockWorkspaceResource, agents: [MockWorkspaceAgentStarting] },
				],
			},
		});
	},
	decorators: [withWebSocket],
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(
					[
						"\x1b[91mCloning Git repository...",
						"\x1b[2;37;41mStarting Docker Daemon...",
						"\x1b[1;95mAdding some 🧙magic🧙...",
						"Starting VS Code...",
						"\r  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0\r100  1475    0  1475    0     0   4231      0 --:--:-- --:--:-- --:--:--  4238",
					].map((line, index) => ({
						id: index,
						level: "info",
						output: line,
						source_id: MockWorkspaceAgentLogSource.id,
						created_at: new Date("2024-01-01T12:00:00Z").toISOString(),
					})),
				),
			},
		],
	},
};

export const StartupScriptError: Story = {
	decorators: [withWebSocket],
	parameters: {
		queries: [
			{
				key: ["tasks", MockTask.owner_name, MockTask.id],
				data: MockTask,
			},
			{
				key: [
					"workspace",
					MockTask.owner_name,
					MockTask.workspace_name,
					"settings",
				],
				data: {
					...MockWorkspace,
					latest_build: {
						...MockWorkspace.latest_build,
						has_ai_task: true,
						resources: [
							{
								...MockWorkspaceResource,
								agents: [MockWorkspaceAgentStartError],
							},
						],
					},
				},
			},
		],
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(
					[
						"Cloning Git repository...",
						"Starting application...",
						"\x1b[91mError: Failed to connect to database",
						"\x1b[91mStartup script exited with code 1",
					].map((line, index) => ({
						id: index,
						level: index >= 2 ? "error" : "info",
						output: line,
						source_id: MockWorkspaceAgentLogSource.id,
						created_at: new Date("2024-01-01T12:00:00Z").toISOString(),
					})),
				),
			},
		],
	},
};

export const StartupScriptTimeout: Story = {
	decorators: [withWebSocket],
	parameters: {
		queries: [
			{
				key: ["tasks", MockTask.owner_name, MockTask.id],
				data: MockTask,
			},
			{
				key: [
					"workspace",
					MockTask.owner_name,
					MockTask.workspace_name,
					"settings",
				],
				data: {
					...MockWorkspace,
					latest_build: {
						...MockWorkspace.latest_build,
						has_ai_task: true,
						resources: [
							{
								...MockWorkspaceResource,
								agents: [MockWorkspaceAgentStartTimeout],
							},
						],
					},
				},
			},
		],
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(
					[
						"Cloning Git repository...",
						"Starting application...",
						"Waiting for dependencies...",
						"Still waiting...",
						"\x1b[93mWarning: Startup script exceeded timeout limit",
					].map((line, index) => ({
						id: index,
						level: index === 4 ? "warn" : "info",
						output: line,
						source_id: MockWorkspaceAgentLogSource.id,
						created_at: new Date("2024-01-01T12:00:00Z").toISOString(),
					})),
				),
			},
		],
	},
};

export const SidebarAppNotFound: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			MockClaudeCodeApp,
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue({
			...task,
			workspace_app_id: null,
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppHealthDisabled: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "disabled" },
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppInitializing: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "initializing" },
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppHealthy: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "healthy" },
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppUnhealthy: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "unhealthy" },
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

const mainAppHealthStory = (health: WorkspaceApp["health"]) => ({
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(MockClaudeCodeApp, {
			...MockVSCodeApp,
			health,
		});
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
});

export const MainAppHealthy: Story = mainAppHealthStory("healthy");
export const MainAppInitializing: Story = mainAppHealthStory("initializing");
export const MainAppUnhealthy: Story = mainAppHealthStory("unhealthy");

export const Active: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			MockClaudeCodeApp,
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const vscodeIframe = await canvas.findByTitle("VS Code Web");
		const zedIframe = await canvas.findByTitle("Zed");
		const claudeIframe = await canvas.findByTitle("Claude Code");

		expect(vscodeIframe).toBeVisible();
		expect(zedIframe).not.toBeVisible();
		expect(claudeIframe).toBeVisible();
	},
};

export const ActivePreview: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			MockClaudeCodeApp,
			MockVSCodeApp,
		);
		spyOn(API.experimental, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = await canvas.findByText("Preview", { exact: false });
		userEvent.click(button);
	},
};

export const WorkspaceStarting: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "startWorkspace").mockResolvedValue(
			MockStartingWorkspace.latest_build,
		);
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: MockStoppedWorkspace.owner_name,
					taskId: MockTask.id,
				},
			},
			routing: {
				path: "/tasks/:username/:taskId",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const startButton = await canvas.findByText("Start workspace");
		expect(startButton).toBeInTheDocument();

		await userEvent.click(startButton);

		await waitFor(async () => {
			expect(API.startWorkspace).toBeCalled();
		});
	},
};

export const WorkspaceStartFailure: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "startWorkspace").mockRejectedValue(
			new Error("Some unexpected error"),
		);
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: MockStoppedWorkspace.owner_name,
					taskId: MockTask.id,
				},
			},
			routing: {
				path: "/tasks/:username/:taskId",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const startButton = await canvas.findByText("Start workspace");
		expect(startButton).toBeInTheDocument();

		await userEvent.click(startButton);

		await waitFor(async () => {
			const errorMessage = await canvas.findByText("Some unexpected error");
			expect(errorMessage).toBeInTheDocument();
		});
	},
};

export const WorkspaceStartFailureWithDialog: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "startWorkspace").mockRejectedValue({
			...mockApiError({
				message: "Bad Request",
				detail: "Invalid build parameters provided",
			}),
			code: "ERR_BAD_REQUEST",
		});
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: MockStoppedWorkspace.owner_name,
					taskId: MockTask.id,
				},
			},
			routing: {
				path: "/tasks/:username/:taskId",
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const startButton = await canvas.findByText("Start workspace");
		expect(startButton).toBeInTheDocument();

		await userEvent.click(startButton);

		await waitFor(async () => {
			const body = within(canvasElement.ownerDocument.body);
			const dialogTitle = await body.findByText("Error building workspace");
			expect(dialogTitle).toBeInTheDocument();
		});
	},
};

function mockTaskWithWorkspace(
	sidebarApp: WorkspaceApp,
	activeApp: WorkspaceApp,
): [Task, Workspace] {
	return [
		{
			...MockTask,
			workspace_app_id: sidebarApp.id,
		},
		{
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				has_ai_task: true,
				resources: [
					{
						...MockWorkspaceResource,
						agents: [
							{
								...MockWorkspaceAgentReady,
								apps: [
									sidebarApp,
									activeApp,
									{
										...MockWorkspaceApp,
										slug: "zed",
										id: "zed",
										display_name: "Zed",
										icon: "/icon/zed.svg",
										health: "healthy",
									},
									{
										...MockWorkspaceApp,
										slug: "preview",
										id: "preview",
										display_name: "Preview",
										health: "healthy",
									},
									{
										...MockWorkspaceApp,
										slug: "disabled",
										id: "disabled",
										display_name: "Disabled",
									},
								],
							},
						],
					},
				],
			},
		},
	];
}
