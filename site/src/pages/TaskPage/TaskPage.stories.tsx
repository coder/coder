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
import type {
	Workspace,
	WorkspaceApp,
	WorkspaceResource,
} from "api/typesGenerated";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import TaskPage, { data, WorkspaceDoesNotHaveAITaskError } from "./TaskPage";

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

export const WaitingOnStatus: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_app_status: null,
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{ ...MockWorkspaceResource, agents: [MockWorkspaceAgentReady] },
					],
				},
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

export const SidebarAppHealthDisabled: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					has_ai_task: true,
					ai_task_sidebar_app_id: "claude-code",
					resources: mockResources({
						claudeCodeAppOverrides: {
							health: "disabled",
						},
					}),
				},
			},
		});
	},
};

export const SidebarAppLoading: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					has_ai_task: true,
					ai_task_sidebar_app_id: "claude-code",
					resources: mockResources({
						claudeCodeAppOverrides: {
							health: "initializing",
						},
					}),
				},
			},
		});
	},
};

export const SidebarAppHealthy: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					has_ai_task: true,
					ai_task_sidebar_app_id: "claude-code",
					resources: mockResources({
						claudeCodeAppOverrides: {
							health: "healthy",
						},
					}),
				},
			},
		});
	},
};

const mainAppHealthStory = (health: WorkspaceApp["health"]) => ({
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: {
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					resources: mockResources({
						claudeCodeAppOverrides: {
							health,
						},
					}),
				},
			},
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

interface MockResourcesProps {
	apps?: WorkspaceApp[];
	claudeCodeAppOverrides?: Partial<WorkspaceApp>;
}

const mockResources = (
	props?: MockResourcesProps,
): readonly WorkspaceResource[] => [
	{
		...MockWorkspaceResource,
		agents: [
			{
				...MockWorkspaceAgentReady,
				apps: [
					...(props?.apps ?? []),
					{
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
						...(props?.claudeCodeAppOverrides ?? {}),
					},
					{
						...MockWorkspaceApp,
						id: "vscode",
						slug: "vscode",
						display_name: "VS Code Web",
						icon: "/icon/code.svg",
					},
					{
						...MockWorkspaceApp,
						slug: "zed",
						id: "zed",
						display_name: "Zed",
						icon: "/icon/zed.svg",
					},
				],
			},
		],
	},
];

const activeWorkspace = (apps: WorkspaceApp[]): Workspace => {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: mockResources({ apps }),
		},
		latest_app_status: {
			...MockWorkspaceAppStatus,
			app_id: "claude-code",
		},
	};
};

export const Active: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: activeWorkspace([]),
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const vscodeIframe = await canvas.findByTitle("VS Code Web");
		const zedIframe = await canvas.findByTitle("Zed");
		const claudeIframe = await canvas.findByTitle("Claude Code");

		expect(vscodeIframe).not.toBeVisible();
		expect(zedIframe).not.toBeVisible();
		expect(claudeIframe).toBeVisible();
	},
};

export const ActivePreview: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: activeWorkspace([
				{
					...MockWorkspaceApp,
					slug: "preview",
					id: "preview",
					display_name: "Preview",
				},
			]),
		});
	},
};

export const WorkspaceStartFailure: Story = {
	decorators: [withGlobalSnackbar],
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStoppedWorkspace,
		});

		spyOn(API, "getWorkspaceParameters").mockResolvedValue({
			templateVersionRichParameters: [],
			buildParameters: [],
		});

		spyOn(API, "startWorkspace").mockRejectedValue(
			new Error("Some unexpected error"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const startButton = await canvas.findByText("Start workspace");
		expect(startButton).toBeInTheDocument();

		await userEvent.click(startButton);

		await waitFor(async () => {
			const errorMessage = await canvas.findByText("Failed to start workspace");
			expect(errorMessage).toBeInTheDocument();
		});
	},
};

export const WorkspaceStartFailureWithDialog: Story = {
	beforeEach: () => {
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStoppedWorkspace,
		});

		spyOn(API, "getWorkspaceParameters").mockResolvedValue({
			templateVersionRichParameters: [],
			buildParameters: [],
		});

		spyOn(API, "startWorkspace").mockRejectedValue({
			...mockApiError({
				message: "Bad Request",
				detail: "Invalid build parameters provided",
			}),
			code: "ERR_BAD_REQUEST",
		});
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
