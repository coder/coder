import type { Meta, StoryObj } from "@storybook/react";
import { expect, spyOn, within } from "@storybook/test";
import { API } from "api/api";
import type {
	Workspace,
	WorkspaceApp,
	WorkspaceResource,
} from "api/typesGenerated";
import {
	MockFailedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
	MockWorkspaceResource,
	mockApiError,
} from "testHelpers/entities";
import { withProxyProvider } from "testHelpers/storybook";
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
			() => new Promise((res) => 1000 * 60 * 60),
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

export const WaitingOnBuildWithTemplate: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		spyOn(data, "fetchTask").mockResolvedValue({
			prompt: "Create competitors page",
			workspace: MockStartingWorkspace,
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
			},
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
				...MockWorkspaceAgent,
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
