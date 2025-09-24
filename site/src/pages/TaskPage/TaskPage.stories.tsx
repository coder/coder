import {
	MockFailedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceAgentLogSource,
	MockWorkspaceAgentReady,
	MockWorkspaceAgentStarting,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceResource,
	mockApiError,
} from "testHelpers/entities";
import {
	withGlobalSnackbar,
	withProxyProvider,
	withWebSocket,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type { Workspace, WorkspaceApp } from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import TaskPage, { data, WorkspaceDoesNotHaveAITaskError } from "./TaskPage";

const MockClaudeCodeApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "claude-code",
	display_name: "Claude Code",
	slug: "claude-code",
	icon: "/icon/claude.svg",
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
};

const meta: Meta<typeof TaskPage> = {
	title: "pages/TaskPage",
	component: TaskPage,
	decorators: [withProxyProvider()],
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof TaskPage>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockImplementation(
			() => new Promise((_res) => 1000 * 60 * 60),
		);
	},
};

export const LoadingError: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockRejectedValue(
			mockApiError({
				message: "Failed to load task",
				detail: "You don't have permission to access this resource.",
			}),
		);
	},
};

export const WaitingOnBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStartingWorkspace,
		});
	},
};

export const FailedBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockFailedWorkspace,
		});
	},
};

export const TerminatedBuild: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStoppedWorkspace,
		});
	},
};

export const TerminatedBuildWithStatus: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockStoppedWorkspace,
				latest_app_status: MockWorkspaceAppStatus,
			},
		});
	},
};

export const WaitingStartupScripts: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					has_ai_task: true,
					resources: [
						{ ...MockWorkspaceResource, agents: [MockWorkspaceAgentStarting] },
					],
				},
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

export const SidebarAppNotFound: Story = {
	beforeEach: () => {
		const workspace = mockTaskWorkspace(MockClaudeCodeApp, MockVSCodeApp);
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...workspace,
				latest_build: {
					...workspace.latest_build,
					ai_task_sidebar_app_id: "non-existent-app-id",
				},
			},
		});
	},
};

export const SidebarAppHealthDisabled: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(
				{
					...MockClaudeCodeApp,
					health: "disabled",
				},
				MockVSCodeApp,
			),
		});
	},
};

export const SidebarAppInitializing: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(
				{
					...MockClaudeCodeApp,
					health: "initializing",
				},
				MockVSCodeApp,
			),
		});
	},
};

export const SidebarAppHealthy: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(
				{
					...MockClaudeCodeApp,
					health: "healthy",
				},
				MockVSCodeApp,
			),
		});
	},
};

export const SidebarAppUnhealthy: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(
				{
					...MockClaudeCodeApp,
					health: "unhealthy",
				},
				MockVSCodeApp,
			),
		});
	},
};

const mainAppHealthStory = (health: WorkspaceApp["health"]) => ({
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(MockClaudeCodeApp, {
				...MockVSCodeApp,
				health,
			}),
		});
	},
});

export const MainAppHealthy: Story = mainAppHealthStory("healthy");
export const MainAppInitializing: Story = mainAppHealthStory("initializing");
export const MainAppUnhealthy: Story = mainAppHealthStory("unhealthy");
export const MainAppHealthDisabled: Story = mainAppHealthStory("disabled");
export const MainAppHealthUnknown: Story = mainAppHealthStory(
	"unknown" as unknown as WorkspaceApp["health"],
);

export const BuildNoAITask: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockImplementation(() => {
			throw new WorkspaceDoesNotHaveAITaskError(MockWorkspace);
		});
	},
};

export const Active: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(MockClaudeCodeApp, MockVSCodeApp),
		});
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
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: mockTaskWorkspace(MockClaudeCodeApp, MockVSCodeApp),
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = await canvas.findByText("Preview", { exact: false });
		userEvent.click(button);
	},
};

export const WorkspaceStartFailure: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(API, "startWorkspace").mockRejectedValue(
			new Error("Some unexpected error"),
		);
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: MockStoppedWorkspace.owner_name,
					workspace: MockStoppedWorkspace.name,
				},
			},
			routing: {
				path: "/tasks/:username/:workspace",
			},
		}),
		queries: [
			{
				key: [
					"tasks",
					MockStoppedWorkspace.owner_name,
					MockStoppedWorkspace.name,
				],
				data: {
					prompt: "Create competitors page",
					workspace: MockStoppedWorkspace,
				},
			},
			{
				key: ["workspace", MockStoppedWorkspace.id, "parameters"],
				data: {
					templateVersionRichParameters: [],
					buildParameters: [],
				},
			},
		],
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
					workspace: MockStoppedWorkspace.name,
				},
			},
			routing: {
				path: "/tasks/:username/:workspace",
			},
		}),
		queries: [
			{
				key: [
					"tasks",
					MockStoppedWorkspace.owner_name,
					MockStoppedWorkspace.name,
				],
				data: {
					prompt: "Create competitors page",
					workspace: MockStoppedWorkspace,
				},
			},
			{
				key: ["workspace", MockStoppedWorkspace.id, "parameters"],
				data: {
					templateVersionRichParameters: [],
					buildParameters: [],
				},
			},
		],
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

function mockTaskWorkspace(
	sidebarApp: WorkspaceApp,
	activeApp: WorkspaceApp,
): Workspace {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			has_ai_task: true,
			ai_task_sidebar_app_id: sidebarApp.id,
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
								},
								{
									...MockWorkspaceApp,
									slug: "preview",
									id: "preview",
									display_name: "Preview",
								},
							],
						},
					],
				},
			],
		},
	};
}
