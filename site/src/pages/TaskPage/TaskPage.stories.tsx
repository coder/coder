import {
	MockCanceledWorkspace,
	MockCancelingWorkspace,
	MockDeletedWorkspace,
	MockDeletingWorkspace,
	MockDisplayNameTasks,
	MockFailedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockStoppingWorkspace,
	MockTask,
	MockTasks,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogSource,
	MockWorkspaceAgentReady,
	MockWorkspaceAgentStarting,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceBuildStop,
	MockWorkspaceResource,
	mockApiError,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
	withProxyProvider,
	withWebSocket,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { taskLogsKey } from "api/queries/tasks";
import type {
	Task,
	TaskLogsResponse,
	Workspace,
	WorkspaceApp,
} from "api/typesGenerated";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
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

const MockTaskLogsResponse: TaskLogsResponse = {
	logs: [
		{
			id: 1,
			content: "Implement JWT authentication with refresh token rotation.",
			type: "input",
			time: "2024-01-01T11:59:55Z",
		},
		{
			id: 2,
			content:
				"I'll help you implement the authentication system. Let me start by examining the existing code structure.",
			type: "output",
			time: "2024-01-01T12:00:00Z",
		},
		{
			id: 3,
			content:
				"Looking at the codebase, I can see the following relevant files:\n- src/auth/login.ts\n- src/auth/middleware.ts\n- src/models/user.ts",
			type: "output",
			time: "2024-01-01T12:00:05Z",
		},
		{
			id: 4,
			content:
				"I'll now create the JWT token validation middleware. This will intercept all protected routes and verify the bearer token.",
			type: "output",
			time: "2024-01-01T12:00:10Z",
		},
		{
			id: 5,
			content:
				"Looks good so far. Also add rate limiting to the token endpoint.",
			type: "input",
			time: "2024-01-01T12:00:12Z",
		},
		{
			id: 6,
			content:
				"Successfully updated src/auth/middleware.ts with the new token validation logic.\nRunning tests to verify the changes...",
			type: "output",
			time: "2024-01-01T12:00:15Z",
		},
		{
			id: 7,
			content:
				"All 12 tests passed. The authentication middleware is working correctly.\n\nNext, I'll add the refresh token rotation endpoint to prevent token reuse attacks.",
			type: "output",
			time: "2024-01-01T12:00:20Z",
		},
	],
	snapshot: true,
	snapshot_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
};

const meta: Meta<typeof TaskPage> = {
	title: "pages/TaskPage",
	component: TaskPage,
	decorators: [withProxyProvider(), withAuthProvider, withDashboardProvider],
	beforeEach: () => {
		spyOn(API, "getTasks").mockResolvedValue(MockTasks);
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
		spyOn(API, "getTask").mockImplementation(() => new Promise(() => {}));
	},
	play: async () => {
		await waitFor(() => {
			expect(API.getTask).toHaveBeenCalledWith(
				MockTask.owner_name,
				MockTask.id,
			);
		});
	},
};

export const LoadingWorkspace: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};

export const LoadingTaskError: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockRejectedValue(
			mockApiError({
				message: "Failed to load task",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const LoadingWorkspaceError: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
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
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStartingWorkspace,
		);
	},
};

export const FailedBuild: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockFailedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const FailedBuildNoSnapshot: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockFailedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue({
			snapshot: true,
			logs: [],
		});
	},
};

export const TerminatedBuild: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const TerminatedBuildWithStatus: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue({
			...MockStoppedWorkspace,
			latest_app_status: MockWorkspaceAppStatus,
		});
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const DeletedWorkspace: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockDeletedWorkspace,
		);
	},
};

export const TaskPausing: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "active",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppingWorkspace,
		);
	},
};

export const TaskPaused: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const TaskPausedNoSnapshot: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue({
			snapshot: true,
			logs: [],
		});
	},
};

export const TaskPausedEmptySnapshot: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue({
			snapshot: true,
			snapshot_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
			logs: [],
		});
	},
};

export const TaskPausedSingleMessage: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue({
			snapshot: true,
			snapshot_at: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
			logs: [MockTaskLogsResponse.logs[0]],
		});
	},
};

export const TaskPausedSnapshotTooltip: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const tooltipTrigger = await canvas.findByRole("button", {
			name: /info/i,
		});
		await userEvent.hover(tooltipTrigger);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toHaveTextContent(
				/This log snapshot was taken/,
			),
		);
	},
};

export const TaskPausedTimeout: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue({
			...MockStoppedWorkspace,
			latest_build: {
				...MockWorkspaceBuildStop,
				status: "stopped",
				reason: "task_auto_pause",
			},
		});
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const TaskCanceled: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockCanceledWorkspace,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const TaskCanceling: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockCancelingWorkspace,
		);
	},
};

export const TaskDeleting: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockDeletingWorkspace,
		);
	},
};

export const WaitingStartupScripts: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
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
				data: {
					...MockTask,
					workspace_agent_lifecycle: "start_error",
				},
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
								agents: [MockWorkspaceAgent],
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
				data: {
					...MockTask,
					workspace_agent_lifecycle: "start_timeout",
				},
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
								agents: [MockWorkspaceAgent],
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
		spyOn(API, "getTask").mockResolvedValue({
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
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppInitializing: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "initializing" },
			MockVSCodeApp,
		);
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppHealthy: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "healthy" },
			MockVSCodeApp,
		);
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

export const SidebarAppUnhealthy: Story = {
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			{ ...MockClaudeCodeApp, health: "unhealthy" },
			MockVSCodeApp,
		);
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
};

const mainAppHealthStory = (health: WorkspaceApp["health"]) => ({
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(MockClaudeCodeApp, {
			...MockVSCodeApp,
			health,
		});
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
});

export const MainAppHealthy: Story = mainAppHealthStory("healthy");
export const MainAppInitializing: Story = mainAppHealthStory("initializing");
export const MainAppUnhealthy: Story = mainAppHealthStory("unhealthy");

export const TaskPausedOutdated: Story = {
	// Given: an 'outdated' workspace (that is, the latest build does not use template's active version)
	parameters: {
		queries: [
			{
				key: ["tasks", { owner: MockTask.owner_name }],
				data: [MockTask],
			},
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
					...MockStoppedWorkspace,
					outdated: true,
				},
			},
			{
				key: [
					"workspaceBuilds",
					MockStoppedWorkspace.latest_build.id,
					"parameters",
				],
				data: [],
			},
			{
				key: taskLogsKey(MockTask.owner_name, MockTask.id),
				data: MockTaskLogsResponse,
			},
		],
	},
	// Then: a tooltip should be displayed prompting the user to update the workspace.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const outdatedTooltip = await canvas.findByTestId(
			"workspace-outdated-tooltip",
		);
		expect(outdatedTooltip).toBeVisible();
	},
};

export const Active: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		const [task, workspace] = mockTaskWithWorkspace(
			MockClaudeCodeApp,
			MockVSCodeApp,
		);
		spyOn(API, "getTask").mockResolvedValue(task);
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
		spyOn(API, "getTask").mockResolvedValue(task);
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = await canvas.findByText("Preview", { exact: false });
		userEvent.click(button);
	},
};

export const TaskResuming: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "startWorkspace").mockResolvedValue(
			MockStartingWorkspace.latest_build,
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
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

		const resumeButton = await canvas.findByText("Resume");
		expect(resumeButton).toBeInTheDocument();

		await userEvent.click(resumeButton);

		await waitFor(async () => {
			expect(API.startWorkspace).toBeCalled();
		});
	},
};

export const TaskResumeFailure: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue({
			...MockTask,
			status: "paused",
		});
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockStoppedWorkspace,
		);
		spyOn(API, "startWorkspace").mockRejectedValue(
			new Error("Some unexpected error"),
		);
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
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

		const resumeButton = await canvas.findByText("Resume");
		expect(resumeButton).toBeInTheDocument();

		await userEvent.click(resumeButton);

		await waitFor(async () => {
			const errorMessage = await canvas.findByText("Some unexpected error");
			expect(errorMessage).toBeInTheDocument();
		});
	},
};

export const TaskResumeFailureWithDialog: Story = {
	beforeEach: () => {
		spyOn(API, "getTask").mockResolvedValue(MockTask);
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
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
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

		const resumeButton = await canvas.findByText("Resume");
		expect(resumeButton).toBeInTheDocument();

		await userEvent.click(resumeButton);

		await waitFor(async () => {
			const body = within(canvasElement.ownerDocument.body);
			const dialogTitle = await body.findByText("Error building workspace");
			expect(dialogTitle).toBeInTheDocument();
		});
	},
};

const longDisplayName =
	"Implement comprehensive authentication and authorization system with role-based access control";
export const LongDisplayName: Story = {
	parameters: {
		queries: [
			{
				// Sidebar: uses getTasks() which returns an array
				key: ["tasks", { owner: MockTask.owner_name }],
				data: [
					{ ...MockDisplayNameTasks[0], display_name: longDisplayName },
					...MockDisplayNameTasks.slice(1),
				],
			},
			{
				// TaskTopbar: uses getTask() which returns a single task
				key: ["tasks", MockTask.owner_name, MockTask.id],
				data: { ...MockDisplayNameTasks[0], display_name: longDisplayName },
			},
			{
				// Workspace data for the task
				key: [
					"workspace",
					MockTask.owner_name,
					MockTask.workspace_name,
					"settings",
				],
				data: MockWorkspace,
			},
		],
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
